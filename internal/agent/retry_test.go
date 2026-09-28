package agent

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"metrics-alerting/internal/retry"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSendRetriesDialWithSameBody(t *testing.T) {
	a := New("localhost:8080")
	a.retryPolicy = retry.Policy{Wait: func(context.Context, time.Duration) error { return nil }}
	calls := 0
	var first string
	a.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if calls == 1 {
			first = string(body)
		}
		if string(body) != first || len(body) == 0 {
			t.Error("request body was not restored")
		}
		if calls < 4 {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
	})
	if err := a.sendPayload(context.Background(), "/updates/", []int{1, 2}); err != nil {
		t.Fatal(err)
	}
	if calls != 4 {
		t.Fatal(calls)
	}
}

func TestSendDoesNotRetryHTTPError(t *testing.T) {
	a := New("localhost:8080")
	calls := 0
	a.client.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusInternalServerError, Header: make(http.Header), Body: http.NoBody}, nil
	})
	if err := a.sendPayload(context.Background(), "/updates/", []int{1}); err == nil || calls != 1 {
		t.Fatal(err, calls)
	}
}
