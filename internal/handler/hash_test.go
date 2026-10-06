package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"metrics-alerting/internal/signature"
)

func TestHashMiddleware(t *testing.T) {
	const key = "secret"
	body := []byte(`{"value":42}`)
	handler := HashMiddleware(key)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, body) {
			t.Fatalf("handler body = %q, want %q", got, body)
		}
		_, _ = w.Write([]byte("response"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set(signature.Header, signature.Calculate(body, key))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got, want := response.Header().Get(signature.Header), signature.Calculate(response.Body.Bytes(), key); got != want {
		t.Fatalf("response signature = %q, want %q", got, want)
	}
}

func TestHashMiddlewareRejectsInvalidSignature(t *testing.T) {
	called := false
	handler := HashMiddleware("secret")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("payload")))
	req.Header.Set(signature.Header, "invalid")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if called {
		t.Fatal("next handler was called for an invalid signature")
	}
}

func TestHashMiddlewareAllowsRequestWithoutSignature(t *testing.T) {
	const key = "secret"
	called := false
	handler := HashMiddleware(key)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"value":42}`)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	if !called {
		t.Fatal("next handler was not called for a request without a signature")
	}
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got, want := response.Header().Get(signature.Header), signature.Calculate(response.Body.Bytes(), key); got != want {
		t.Fatalf("response signature = %q, want %q", got, want)
	}
}
