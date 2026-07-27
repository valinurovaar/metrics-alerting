package agent

import (
	"fmt"
	"math/rand"
	"net/http"
	"runtime"
	"strconv"
	"time"
)

const (
	PollInterval   = 2 * time.Second
	ReportInterval = 10 * time.Second
	DefaultAddress = "http://localhost:8080"
)

type Agent struct {
	serverURL string
	client    *http.Client
	gauges    map[string]float64
	counters  map[string]int64
}

func New(serverURL string) *Agent {
	return &Agent{
		serverURL: serverURL,
		client:    &http.Client{},
		gauges:    make(map[string]float64),
		counters:  make(map[string]int64),
	}
}

func (a *Agent) Run() {
	go func() {
		for {
			a.Poll()
			time.Sleep(PollInterval)
		}
	}()

	for {
		a.Report()
		time.Sleep(ReportInterval)
	}
}

func (a *Agent) Poll() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

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

func (a *Agent) Report() {
	for name, value := range a.gauges {
		a.sendMetric("gauge", name, strconv.FormatFloat(value, 'f', -1, 64))
	}

	for name, value := range a.counters {
		if value == 0 {
			continue
		}
		a.sendMetric("counter", name, strconv.FormatInt(value, 10))
		a.counters[name] = 0
	}
}

func (a *Agent) sendMetric(mType, name, value string) {
	url := fmt.Sprintf("%s/update/%s/%s/%s", a.serverURL, mType, name, value)

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := a.client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}
