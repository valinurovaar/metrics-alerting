package agent

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
		"encoding/json"
	"io"

	"metrics-alerting/internal/model"
)

func TestPoll_CollectsRuntimeMetrics(t *testing.T) {
	a := New("http://localhost:8080")

	a.Poll()

	expectedGauges := []string{
		"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys",
		"HeapAlloc", "HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased",
		"HeapSys", "LastGC", "Lookups", "MCacheInuse", "MCacheSys",
		"MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC",
		"NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys",
		"Sys", "TotalAlloc", "RandomValue",
	}

	for _, name := range expectedGauges {
		if _, ok := a.gauges[name]; !ok {
			t.Errorf("Expected gauge metric %q to be collected", name)
		}
	}
}

func TestPoll_IncrementsPollCount(t *testing.T) {
	a := New("http://localhost:8080")

	a.Poll()
	if a.counters["PollCount"] != 1 {
		t.Errorf("Expected PollCount=1 after first poll, got %d", a.counters["PollCount"])
	}

	a.Poll()
	if a.counters["PollCount"] != 2 {
		t.Errorf("Expected PollCount=2 after second poll, got %d", a.counters["PollCount"])
	}
}

func TestReport_SendsMetrics(t *testing.T) {
	var (
		mu       sync.Mutex
		received []model.Metrics
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", ct)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}

		var metric model.Metrics
		if err := json.Unmarshal(body, &metric); err != nil {
			t.Fatalf("cannot decode json: %v", err)
		}

		mu.Lock()
		received = append(received, metric)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := New(server.URL)

	a.Poll()
	a.Report()

	mu.Lock()
	defer mu.Unlock()

	if len(received) == 0 {
		t.Fatal("Expected metrics to be sent")
	}

	var (
		hasAlloc     bool
		hasRandom    bool
		hasPollCount bool
	)

	for _, metric := range received {
		switch metric.ID {

		case "Alloc":
			if metric.MType != "gauge" || metric.Value == nil {
				t.Errorf("Alloc metric is invalid: %+v", metric)
			}
			hasAlloc = true

		case "RandomValue":
			if metric.MType != "gauge" || metric.Value == nil {
				t.Errorf("RandomValue metric is invalid: %+v", metric)
			}
			hasRandom = true

		case "PollCount":
			if metric.MType != "counter" || metric.Delta == nil || *metric.Delta != 1 {
				t.Errorf("PollCount metric is invalid: %+v", metric)
			}
			hasPollCount = true
		}
	}

	if !hasAlloc {
		t.Error("Alloc metric was not sent")
	}

	if !hasRandom {
		t.Error("RandomValue metric was not sent")
	}

	if !hasPollCount {
		t.Error("PollCount metric was not sent")
	}
}

func TestReport_ResetsCounterAfterSend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := New(server.URL)
	a.Poll()
	a.Poll()
	a.Report()

	if a.counters["PollCount"] != 0 {
		t.Errorf("Expected PollCount to reset after report, got %d", a.counters["PollCount"])
	}
}
