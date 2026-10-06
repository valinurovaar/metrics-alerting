package agent

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"metrics-alerting/internal/model"
)

func TestReport_SendsMetrics(t *testing.T) {
	receivedMetrics := make(chan model.Metrics, 100)
	var mu sync.Mutex
	received := make([]model.Metrics, 0)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reader io.Reader = r.Body

		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Errorf("cannot create gzip reader: %v", err)
				http.Error(w, "bad gzip", http.StatusBadRequest)
				return
			}
			defer gz.Close()
			reader = gz
		}

		var req model.Metrics
		if err := json.NewDecoder(reader).Decode(&req); err != nil {
			t.Errorf("cannot decode json: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}

		mu.Lock()
		received = append(received, req)
		mu.Unlock()

		select {
		case receivedMetrics <- req:
		default:
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(req)
	}))
	defer srv.Close()

	a := New(srv.URL)
	a.SetPollInterval(50 * time.Millisecond)
	a.SetReportInterval(100 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go a.Run(ctx)

	deadline := time.After(3 * time.Second)
	expectedCount := 5

	for {
		mu.Lock()
		currentCount := len(received)
		mu.Unlock()

		if currentCount >= expectedCount {
			break
		}

		select {
		case <-receivedMetrics:
		case <-deadline:
			mu.Lock()
			finalCount := len(received)
			mu.Unlock()
			t.Errorf("Expected metrics to be sent, got %d", finalCount)
			return
		}
	}

	hasGauge := false

	mu.Lock()
	for _, m := range received {
		if m.MType == "gauge" {
			hasGauge = true
			break
		}
	}
	mu.Unlock()

	if !hasGauge {
		t.Errorf("Expected at least one gauge metric to be sent")
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "with http prefix",
			input:    "http://localhost:8080",
			expected: "http://localhost:8080",
		},
		{
			name:     "with https prefix",
			input:    "https://example.com",
			expected: "https://example.com",
		},
		{
			name:     "without prefix",
			input:    "localhost:8080",
			expected: "http://localhost:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(tt.input)
			if a.serverURL != tt.expected {
				t.Errorf("expected serverURL=%s, got %s", tt.expected, a.serverURL)
			}
			if a.client == nil {
				t.Errorf("expected client to be initialized")
			}
		})
	}
}

func TestAgent_Intervals(t *testing.T) {
	a := New("http://localhost:8080")

	a.SetReportInterval(30 * time.Second)
	if a.reportInterval != 30*time.Second {
		t.Errorf("expected reportInterval=30s, got %v", a.reportInterval)
	}

	a.SetPollInterval(5 * time.Second)
	if a.pollInterval != 5*time.Second {
		t.Errorf("expected pollInterval=5s, got %v", a.pollInterval)
	}
}