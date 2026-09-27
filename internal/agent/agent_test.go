package agent

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"metrics-alerting/internal/model"
	"metrics-alerting/internal/signature"
)

func TestSendRequestSignsBody(t *testing.T) {
	const key = "secret"
	body := []byte("compressed request body")
	var receivedHash string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHash = r.Header.Get(signature.Header)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := New(srv.URL)
	a.SetKey(key)
	if err := a.sendRequest(context.Background(), srv.URL, body); err != nil {
		t.Fatal(err)
	}
	if want := signature.Calculate(body, key); receivedHash != want {
		t.Fatalf("request signature = %q, want %q", receivedHash, want)
	}
}

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

		if r.URL.Path != "/updates/" || r.Header.Get("Content-Encoding") != "gzip" {
			t.Errorf("expected gzip batch API, got %s", r.URL.Path)
		}
		var req []model.Metrics
		if err := json.NewDecoder(reader).Decode(&req); err != nil {
			t.Errorf("cannot decode json: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}

		mu.Lock()
		received = append(received, req...)
		mu.Unlock()

		select {
		case receivedMetrics <- req[0]:
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

func TestPollSystem(t *testing.T) {
	a := New("localhost:8080")
	if err := a.PollSystem(); err != nil {
		t.Fatal(err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.gauges["TotalMemory"]; !ok {
		t.Error("TotalMemory was not collected")
	}
	if _, ok := a.gauges["FreeMemory"]; !ok {
		t.Error("FreeMemory was not collected")
	}
	for i := 1; i <= runtime.NumCPU(); i++ {
		if _, ok := a.gauges["CPUutilization"+strconv.Itoa(i)]; !ok {
			t.Errorf("CPUutilization%d was not collected", i)
		}
	}
}

func TestRunRespectsRateLimit(t *testing.T) {
	const limit = 2
	var active atomic.Int32
	var maximum atomic.Int32
	release := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			old := maximum.Load()
			if current <= old || maximum.CompareAndSwap(old, current) {
				break
			}
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := New(srv.URL)
	a.SetRateLimit(limit)
	a.SetPollInterval(time.Hour)
	a.SetReportInterval(10 * time.Millisecond)
	for i := 0; i < limit+3; i++ {
		a.gauges["test"+strconv.Itoa(i)] = float64(i)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.Run(ctx)
		close(done)
	}()

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for maximum.Load() < limit {
		select {
		case <-deadline.C:
			close(release)
			cancel()
			<-done
			t.Fatalf("maximum concurrency = %d, want %d", maximum.Load(), limit)
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if got := maximum.Load(); got > limit {
		t.Errorf("maximum concurrency = %d, exceeds rate limit %d", got, limit)
	}

	close(release)
	cancel()
	<-done
}
