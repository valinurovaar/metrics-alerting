package handler

import (
	"fmt"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
	"go.uber.org/zap"

	"github.com/go-chi/chi/v5"

	"metrics-alerting/internal/model"
	"metrics-alerting/internal/storage"
)

type MetricsServer struct {
	storage storage.Storage
	logger  *zap.Logger
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func NewMetricsServer(s storage.Storage, logger *zap.Logger) *MetricsServer {
	return &MetricsServer{
		storage: s,
		logger:  logger,
	}
}

func (s *MetricsServer) Routes() chi.Router {
	r := chi.NewRouter()

	r.Use(LoggingMiddleware(s.logger))

	r.Post("/update/{type}/{name}/{value}", s.UpdateHandler)

	r.Post("/update", s.UpdateJSONHandler)
	r.Post("/update/", s.UpdateJSONHandler)

	r.Get("/value/{type}/{name}", s.GetValueHandler)

	r.Post("/value", s.PostValueJSONHandler)
	r.Post("/value/", s.PostValueJSONHandler)

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

func (s *MetricsServer) UpdateJSONHandler(w http.ResponseWriter, r *http.Request) {
	var metric model.Metrics

	if err := json.NewDecoder(r.Body).Decode(&metric); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if metric.ID == "" {
		http.Error(w, "metric id is required", http.StatusNotFound)
		return
	}

	if metric.MType != "gauge" && metric.MType != "counter" {
		http.Error(w, "invalid metric type", http.StatusBadRequest)
		return
	}

	if metric.MType == "gauge" && metric.Value == nil {
		http.Error(w, "metric value is required", http.StatusBadRequest)
		return
	}

	if metric.MType == "counter" && metric.Delta == nil {
		http.Error(w, "metric delta is required", http.StatusBadRequest)
		return
	}

	if err := s.storage.Update(&metric); err != nil {
		http.Error(w, "failed to update metric", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(metric); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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

func (s *MetricsServer) PostValueJSONHandler(w http.ResponseWriter, r *http.Request) {
	var req model.Metrics

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		http.Error(w, "metric id is required", http.StatusNotFound)
		return
	}

	if req.MType != "gauge" && req.MType != "counter" {
		http.Error(w, "invalid metric type", http.StatusBadRequest)
		return
	}

	metric, ok := s.storage.GetMetric(req.ID, req.MType)
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(metric); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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

func LoggingMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			ww := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(ww, r)

			logger.Info(
				"HTTP request",
				zap.String("uri", r.RequestURI),
				zap.String("method", r.Method),
				zap.Int("status", ww.statusCode),
				zap.Int("size", ww.size),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(data)
	rw.size += size
	return size, err
}