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
	pollTicker := time.NewTicker(a.pollInterval)
	defer pollTicker.Stop()

	reportTicker := time.NewTicker(a.reportInterval)
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
	a.mu.Lock()

	gaugesCopy := make(map[string]float64, len(a.gauges))
	for k, v := range a.gauges {
		gaugesCopy[k] = v
	}

	countersCopy := make(map[string]int64, len(a.counters))
	for k, v := range a.counters {
		if v != 0 {
			countersCopy[k] = v
			a.counters[k] = 0
		}
	}

	a.mu.Unlock()

	for name, value := range gaugesCopy {
		select {
		case <-ctx.Done():
			return
		default:
		}

		v := value
		a.sendMetric(ctx, model.Metrics{
			ID:    name,
			MType: "gauge",
			Value: &v,
		})
	}

	for name, value := range countersCopy {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if value == 0 {
			continue
		}
		v := value
		a.sendMetric(ctx, model.Metrics{
			ID:    name,
			MType: "counter",
			Delta: &v,
		})
	}
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
	body, err := json.Marshal(metric)
	if err != nil {
		fmt.Printf("marshal error: %v\n", err)
		return
	}

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)

	if _, err := gzipWriter.Write(body); err != nil {
		fmt.Printf("gzip write error: %v\n", err)
		return
	}

	if err := gzipWriter.Close(); err != nil {
		fmt.Printf("gzip close error: %v\n", err)
		return
	}

	endpoint := strings.TrimRight(a.serverURL, "/") + "/update"

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		&buf,
	)
	if err != nil {
		fmt.Printf("create request error: %v\n", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := a.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		fmt.Printf("send request error: %v\n", err)
		return
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
		fmt.Printf("unexpected status: %d\n", resp.StatusCode)
		return
	}
}