package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReportEmptyBatch(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer srv.Close()
	a := New(srv.URL)
	a.counters["C"] = 0
	a.Report(context.Background())
	if requests != 0 {
		t.Fatal("empty batch was sent")
	}
}

func TestReportPreservesCounters(t *testing.T) {
	status := http.StatusInternalServerError
	var a *Agent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.counters["C"]++ // Collection continues while the HTTP request is in flight.
		a.mu.Unlock()
		w.WriteHeader(status)
	}))
	defer srv.Close()
	a = New(srv.URL)
	a.counters["C"] = 5
	a.Report(context.Background())
	if a.counters["C"] != 6 {
		t.Fatalf("failed report lost counters: %d", a.counters["C"])
	}
	status = http.StatusOK
	a.Report(context.Background())
	if a.counters["C"] != 1 {
		t.Fatalf("successful report lost concurrent collection: %d", a.counters["C"])
	}
}
