package agent

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"metrics-alerting/internal/model"
)

func runReport(a *Agent, ctx context.Context) {
	jobs := make(chan reportJob, 1)
	a.enqueueReport(ctx, jobs)
	close(jobs)
	a.reportWorker(ctx, jobs)
}

func TestWorkerSendsWholeBatch(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/updates/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		reader, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		defer reader.Close()
		var metrics []model.Metrics
		if err := json.NewDecoder(reader).Decode(&metrics); err != nil {
			t.Error(err)
		}
		if len(metrics) != 33 {
			t.Errorf("batch contains %d metrics, want 33", len(metrics))
		}
	}))
	defer srv.Close()
	a := New(srv.URL)
	for i := 0; i < 32; i++ {
		a.gauges[strconv.Itoa(i)] = float64(i)
	}
	a.counters["C"] = 5
	runReport(a, context.Background())
	if requests != 1 {
		t.Fatalf("sent %d requests, want one batch", requests)
	}
}

func TestEnqueueCanceledBatchRestoresCounters(t *testing.T) {
	a := New("localhost:8080")
	a.counters["C"] = 5
	a.counters["D"] = 7
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if a.enqueueReport(ctx, make(chan reportJob)) {
		t.Fatal("canceled batch was queued")
	}
	if a.counters["C"] != 5 || a.counters["D"] != 7 {
		t.Fatalf("counters lost: %v", a.counters)
	}
}

func TestReportEmptyBatch(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer srv.Close()
	a := New(srv.URL)
	a.counters["C"] = 0
	runReport(a, context.Background())
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
	runReport(a, context.Background())
	if a.counters["C"] != 6 {
		t.Fatalf("failed report lost counters: %d", a.counters["C"])
	}
	status = http.StatusOK
	runReport(a, context.Background())
	if a.counters["C"] != 1 {
		t.Fatalf("successful report lost concurrent collection: %d", a.counters["C"])
	}
}
