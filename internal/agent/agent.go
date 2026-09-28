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

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"metrics-alerting/internal/model"
	"metrics-alerting/internal/retry"
	"metrics-alerting/internal/signature"
)

const (
	PollInterval   = 2 * time.Second
	ReportInterval = 10 * time.Second
	RateLimit      = 3
)

type Agent struct {
	serverURL      string
	client         *http.Client
	gauges         map[string]float64
	counters       map[string]int64
	reportInterval time.Duration
	pollInterval   time.Duration
	rateLimit      int
	mu             sync.Mutex
	reportMu       sync.Mutex
	retryPolicy    retry.Policy
	key            string
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
		rateLimit:      RateLimit,
	}
}

func (a *Agent) Run(ctx context.Context) {
	a.mu.Lock()
	pollInterval, reportInterval, rateLimit := a.pollInterval, a.reportInterval, a.rateLimit
	a.mu.Unlock()

	jobs := make(chan reportJob, rateLimit*2)
	var wg sync.WaitGroup
	for i := 0; i < rateLimit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.reportWorker(ctx, jobs)
		}()
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		a.collectRuntime(ctx, pollInterval)
	}()
	go func() {
		defer wg.Done()
		a.collectSystem(ctx, pollInterval)
	}()

	reportTicker := time.NewTicker(reportInterval)
	defer reportTicker.Stop()
	defer wg.Wait()

	for {
		select {
		case <-ctx.Done():
			close(jobs)
			return
		case <-reportTicker.C:
			if !a.enqueueReport(ctx, jobs) {
				close(jobs)
				return
			}
		}
	}
}

type reportJob struct {
	metric       model.Metrics
	counterName  string
	counterDelta int64
}

func (a *Agent) collectRuntime(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.Poll()
		}
	}
}

func (a *Agent) collectSystem(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.PollSystem(); err != nil {
				fmt.Printf("collect system metrics error: %v\n", err)
			}
		}
	}
}

func (a *Agent) enqueueReport(ctx context.Context, jobs chan<- reportJob) bool {
	a.mu.Lock()
	report := make([]reportJob, 0, len(a.gauges)+len(a.counters))
	for name, gauge := range a.gauges {
		value := gauge
		report = append(report, reportJob{metric: model.Metrics{ID: name, MType: model.Gauge, Value: &value}})
	}
	for name, counter := range a.counters {
		if counter == 0 {
			continue
		}
		delta := counter
		report = append(report, reportJob{
			metric:       model.Metrics{ID: name, MType: model.Counter, Delta: &delta},
			counterName:  name,
			counterDelta: delta,
		})
		a.counters[name] -= delta
	}
	a.mu.Unlock()

	for i, job := range report {
		select {
		case jobs <- job:
		case <-ctx.Done():
			for _, unsent := range report[i:] {
				a.restoreCounter(unsent)
			}
			return false
		}
	}
	return true
}

func (a *Agent) reportWorker(ctx context.Context, jobs <-chan reportJob) {
	for job := range jobs {
		if err := a.sendPayload(ctx, "/updates/", []model.Metrics{job.metric}); err != nil {
			a.restoreCounter(job)
			if ctx.Err() == nil {
				fmt.Printf("send metric %s error: %v\n", job.metric.ID, err)
			}
		}
	}
}

func (a *Agent) restoreCounter(job reportJob) {
	if job.counterName == "" {
		return
	}
	a.mu.Lock()
	a.counters[job.counterName] += job.counterDelta
	a.mu.Unlock()
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

// PollSystem collects host metrics independently from Go runtime metrics.
func (a *Agent) PollSystem() error {
	memory, err := mem.VirtualMemory()
	if err != nil {
		return fmt.Errorf("read virtual memory: %w", err)
	}
	utilization, err := cpu.Percent(0, true)
	if err != nil {
		return fmt.Errorf("read cpu utilization: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.gauges["TotalMemory"] = float64(memory.Total)
	a.gauges["FreeMemory"] = float64(memory.Free)
	for i, value := range utilization {
		a.gauges[fmt.Sprintf("CPUutilization%d", i+1)] = value
	}
	return nil
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

func (a *Agent) SetRateLimit(limit int) {
	if limit <= 0 {
		limit = 1
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rateLimit = limit
}

func (a *Agent) SetKey(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.key = key
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
	compressed := append([]byte(nil), buf.Bytes()...)
	return a.retryPolicy.Do(ctx, func() error {
		return a.sendRequest(ctx, endpoint, compressed)
	}, retry.IsDialError)
}

func (a *Agent) sendRequest(ctx context.Context, endpoint string, compressed []byte) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(compressed),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")
	a.mu.Lock()
	key := a.key
	a.mu.Unlock()
	if key != "" {
		req.Header.Set(signature.Header, signature.Calculate(compressed, key))
	}

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
