package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"metrics-alerting/internal/model"
)

const (
	PollInterval   = 2 * time.Second
	ReportInterval = 10 * time.Second
)

type Agent struct {
	serverURL      string
	client         *http.Client
	gauges         map[string]float64
	counters       map[string]int64
	reportInterval time.Duration
	pollInterval   time.Duration
	mu             sync.Mutex
	reportMu       sync.Mutex
}

func New(serverURL string) *Agent {
	if !strings.HasPrefix(serverURL, "http://") &&
		!strings.HasPrefix(serverURL, "https://") {
		serverURL = "http://" + serverURL
	}

	return &Agent{
		serverURL: serverURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		gauges:         make(map[string]float64),
		counters:       make(map[string]int64),
		reportInterval: ReportInterval,
		pollInterval:   PollInterval,
	}
}

func (a *Agent) Run(ctx context.Context) {
	a.mu.Lock()
	pollInterval, reportInterval := a.pollInterval, a.reportInterval
	a.mu.Unlock()
	pollTicker := time.NewTicker(pollInterval)
	defer pollTicker.Stop()

	reportTicker := time.NewTicker(reportInterval)
	defer reportTicker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-pollTicker.C:
				a.Poll()
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-reportTicker.C:
			a.Report(ctx)
		}
	}
}

func (a *Agent) Poll() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	a.mu.Lock()
	defer a.mu.Unlock()

	a.gauges["Alloc"] = float64(ms.Alloc)
	a.gauges["BuckHashSys"] = float64(ms.BuckHashSys)
	a.gauges["Frees"] = float64(ms.Frees)
	a.gauges["GCCPUFraction"] = ms.GCCPUFraction
	a.gauges["GCSys"] = float64(ms.GCSys)
	a.gauges["HeapAlloc"] = float64(ms.HeapAlloc)
	a.gauges["HeapIdle"] = float64(ms.HeapIdle)
	a.gauges["HeapInuse"] = float64(ms.HeapInuse)
	a.gauges["HeapObjects"] = float64(ms.HeapObjects)
	a.gauges["HeapReleased"] = float64(ms.HeapReleased)
	a.gauges["HeapSys"] = float64(ms.HeapSys)
	a.gauges["LastGC"] = float64(ms.LastGC)
	a.gauges["Lookups"] = float64(ms.Lookups)
	a.gauges["MCacheInuse"] = float64(ms.MCacheInuse)
	a.gauges["MCacheSys"] = float64(ms.MCacheSys)
	a.gauges["MSpanInuse"] = float64(ms.MSpanInuse)
	a.gauges["MSpanSys"] = float64(ms.MSpanSys)
	a.gauges["Mallocs"] = float64(ms.Mallocs)
	a.gauges["NextGC"] = float64(ms.NextGC)
	a.gauges["NumForcedGC"] = float64(ms.NumForcedGC)
	a.gauges["NumGC"] = float64(ms.NumGC)
	a.gauges["OtherSys"] = float64(ms.OtherSys)
	a.gauges["PauseTotalNs"] = float64(ms.PauseTotalNs)
	a.gauges["StackInuse"] = float64(ms.StackInuse)
	a.gauges["StackSys"] = float64(ms.StackSys)
	a.gauges["Sys"] = float64(ms.Sys)
	a.gauges["TotalAlloc"] = float64(ms.TotalAlloc)

	a.counters["PollCount"]++
	a.gauges["RandomValue"] = rand.Float64()
}

func (a *Agent) Report(ctx context.Context) {
	a.reportMu.Lock()
	defer a.reportMu.Unlock()
	a.mu.Lock()
	metrics := make([]model.Metrics, 0, len(a.gauges)+len(a.counters))
	for k, v := range a.gauges {
		value := v
		metrics = append(metrics, model.Metrics{ID: k, MType: model.Gauge, Value: &value})
	}

	countersCopy := make(map[string]int64, len(a.counters))
	for k, v := range a.counters {
		if v != 0 {
			countersCopy[k] = v
			delta := v
			metrics = append(metrics, model.Metrics{ID: k, MType: model.Counter, Delta: &delta})
		}
	}

	a.mu.Unlock()

	if len(metrics) == 0 || ctx.Err() != nil {
		return
	}
	if err := a.sendPayload(ctx, "/updates/", metrics); err != nil {
		fmt.Printf("send batch error: %v\n", err)
		return
	}
	a.mu.Lock()
	for name, delta := range countersCopy {
		a.counters[name] -= delta
	}
	a.mu.Unlock()
}

func (a *Agent) SetReportInterval(interval time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reportInterval = interval
}

func (a *Agent) SetPollInterval(interval time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pollInterval = interval
}

func (a *Agent) sendMetric(ctx context.Context, metric model.Metrics) {
	if err := a.sendPayload(ctx, "/update", metric); err != nil {
		fmt.Printf("send metric error: %v\n", err)
	}
}

func (a *Agent) sendPayload(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)

	if _, err := gzipWriter.Write(body); err != nil {
		return err
	}

	if err := gzipWriter.Close(); err != nil {
		return err
	}

	endpoint := strings.TrimRight(a.serverURL, "/") + path

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		&buf,
	)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Encoding")), "gzip") {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err == nil {
			defer gzipReader.Close()
			_, _ = io.Copy(io.Discard, gzipReader)
		} else {
			_, _ = io.Copy(io.Discard, resp.Body)
		}
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}
