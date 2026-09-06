package agent

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
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
	var mu sync.Mutex
	received := make(map[string]string)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "text/plain" {
			t.Errorf("Expected Content-Type text/plain, got %s", ct)
		}

		mu.Lock()
		received[r.URL.Path] = r.Method
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := New(server.URL)
	a.Poll()
	a.Report()

	mu.Lock()
	defer mu.Unlock()

	if len(received) == 0 {
		t.Fatal("Expected metrics to be sent to server")
	}

	expectedPaths := []string{
		"/update/gauge/Alloc/",
		"/update/counter/PollCount/1",
		"/update/gauge/RandomValue/",
	}

	for _, prefix := range expectedPaths {
		found := false
		for path := range received {
			if len(prefix) > 0 && prefix[len(prefix)-1] == '/' {
				if len(path) > len(prefix)-1 && path[:len(prefix)-1] == prefix[:len(prefix)-1] {
					found = true
					break
				}
			} else if path == prefix {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected request matching %q, received paths: %v", prefix, received)
		}
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
