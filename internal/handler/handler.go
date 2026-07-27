package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"metrics-alerting/internal/model"
	"metrics-alerting/internal/storage"
)

type MetricsServer struct {
	storage storage.Storage
}

func NewMetricsServer(s storage.Storage) *MetricsServer {
	return &MetricsServer{storage: s}
}

func (s *MetricsServer) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/update/{type}/{name}/{value}", s.UpdateHandler)
	r.Get("/value/{type}/{name}", s.GetValueHandler)
	r.Get("/", s.ListHandler)
	return r
}

func (s *MetricsServer) UpdateHandler(w http.ResponseWriter, r *http.Request) {
	metricType := chi.URLParam(r, "type")
	metricID := chi.URLParam(r, "name")
	metricValueStr := chi.URLParam(r, "value")

	if metricID == "" {
		http.Error(w, "Metric name is required", http.StatusNotFound)
		return
	}

	if metricType != "gauge" && metricType != "counter" {
		http.Error(w, "Invalid metric type", http.StatusBadRequest)
		return
	}

	metric := &model.Metrics{
		ID:    metricID,
		MType: metricType,
	}

	if metricType == "gauge" {
		value, err := strconv.ParseFloat(metricValueStr, 64)
		if err != nil {
			http.Error(w, "Invalid metric value", http.StatusBadRequest)
			return
		}
		metric.Value = &value
	} else {
		delta, err := strconv.ParseInt(metricValueStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid metric value", http.StatusBadRequest)
			return
		}
		metric.Delta = &delta
	}

	if err := s.storage.Update(metric); err != nil {
		http.Error(w, "Failed to update metric", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK\n"))
}

func (s *MetricsServer) GetValueHandler(w http.ResponseWriter, r *http.Request) {
	metricType := chi.URLParam(r, "type")
	metricID := chi.URLParam(r, "name")

	if metricType != "gauge" && metricType != "counter" {
		http.NotFound(w, r)
		return
	}

	metric, ok := s.storage.GetMetric(metricID, metricType)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var valueStr string
	if metricType == "gauge" {
		if metric.Value == nil {
			http.NotFound(w, r)
			return
		}
		valueStr = strconv.FormatFloat(*metric.Value, 'f', -1, 64)
	} else {
		if metric.Delta == nil {
			http.NotFound(w, r)
			return
		}
		valueStr = strconv.FormatInt(*metric.Delta, 10)
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(valueStr))
}

func (s *MetricsServer) ListHandler(w http.ResponseWriter, r *http.Request) {
	metrics := s.storage.GetAllMetrics()

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html><head><title>Metrics</title></head><body>\n")
	b.WriteString("<h1>Metrics</h1>\n<table border=\"1\">\n")
	b.WriteString("<tr><th>Type</th><th>Name</th><th>Value</th></tr>\n")

	for _, m := range metrics {
		var valueStr string
		if m.MType == "gauge" && m.Value != nil {
			valueStr = strconv.FormatFloat(*m.Value, 'f', -1, 64)
		} else if m.MType == "counter" && m.Delta != nil {
			valueStr = strconv.FormatInt(*m.Delta, 10)
		}
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%s</td></tr>\n", m.MType, m.ID, valueStr)
	}

	b.WriteString("</table>\n</body></html>\n")

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(b.String()))
}
