package handler

import (
	"bytes"
	"io"
	"net/http"

	"metrics-alerting/internal/signature"
)

type hashResponseWriter struct {
	header     http.Header
	body       bytes.Buffer
	statusCode int
}

func newHashResponseWriter() *hashResponseWriter {
	return &hashResponseWriter{header: make(http.Header)}
}

func (w *hashResponseWriter) Header() http.Header {
	return w.header
}

func (w *hashResponseWriter) WriteHeader(statusCode int) {
	if w.statusCode == 0 {
		w.statusCode = statusCode
	}
}

func (w *hashResponseWriter) Write(data []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	return w.body.Write(data)
}

// HashMiddleware verifies request bodies and signs response bodies with HMAC-SHA256.
// An empty key disables both operations.
func HashMiddleware(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if key == "" {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			buffered := newHashResponseWriter()

			if r.Body != nil {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(buffered, "failed to read request body", http.StatusBadRequest)
					writeSignedResponse(w, buffered, key)
					return
				}
				_ = r.Body.Close()
				r.Body = io.NopCloser(bytes.NewReader(body))

				if !signature.Valid(body, key, r.Header.Get(signature.Header)) {
					http.Error(buffered, "invalid request hash", http.StatusBadRequest)
					writeSignedResponse(w, buffered, key)
					return
				}
			}

			next.ServeHTTP(buffered, r)
			writeSignedResponse(w, buffered, key)
		})
	}
}

func writeSignedResponse(w http.ResponseWriter, buffered *hashResponseWriter, key string) {
	for name, values := range buffered.header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	body := buffered.body.Bytes()
	w.Header().Set(signature.Header, signature.Calculate(body, key))
	statusCode := buffered.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}
