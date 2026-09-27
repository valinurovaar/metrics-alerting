package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"metrics-alerting/internal/model"
	"metrics-alerting/internal/storage"
)

type MetricsServer struct {
	storage  storage.Storage
	logger   *zap.Logger
	database DatabasePinger
	key      string
}

type DatabasePinger interface {
	PingContext(ctx context.Context) error
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func NewMetricsServer(s storage.Storage, logger *zap.Logger, database ...DatabasePinger) *MetricsServer {
	server := &MetricsServer{
		storage: s,
		logger:  logger,
	}
	if len(database) > 0 {
		server.database = database[0]
	}

	return server
}

func (s *MetricsServer) Routes() chi.Router {
	r := chi.NewRouter()

	r.Use(HashMiddleware(s.key))
	r.Use(GzipMiddleware)
	r.Use(LoggingMiddleware(s.logger))

	r.Post("/update/{type}/{name}/{value}", s.UpdateHandler)

	r.Post("/update", s.UpdateJSONHandler)
	r.Post("/update/", s.UpdateJSONHandler)
	r.Post("/updates", s.UpdateBatchHandler)
	r.Post("/updates/", s.UpdateBatchHandler)

	r.Get("/value/{type}/{name}", s.GetValueHandler)
	r.Get("/ping", s.PingHandler)

	r.Post("/value", s.PostValueJSONHandler)
	r.Post("/value/", s.PostValueJSONHandler)

	r.Get("/", s.ListHandler)

	return r
}

func (s *MetricsServer) SetKey(key string) {
	s.key = key
}

func (s *MetricsServer) UpdateBatchHandler(w http.ResponseWriter, r *http.Request) {
	var metrics []model.Metrics
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&metrics); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	for _, metric := range metrics {
		if metric.ID == "" {
			http.Error(w, "metric id is required", http.StatusNotFound)
			return
		}
		if (metric.MType != model.Gauge && metric.MType != model.Counter) ||
			(metric.MType == model.Gauge && metric.Value == nil) ||
			(metric.MType == model.Counter && metric.Delta == nil) {
			http.Error(w, "invalid metric", http.StatusBadRequest)
			return
		}
	}
	if err := s.storage.UpdateBatch(r.Context(), metrics); err != nil {
		s.logger.Error("failed to update metrics batch", zap.Error(err))
		http.Error(w, "failed to update metrics", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{}"))
}

func (s *MetricsServer) PingHandler(w http.ResponseWriter, r *http.Request) {
	if s.database == nil {
		http.Error(w, "database connection failed", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	if err := s.database.PingContext(ctx); err != nil {
		s.logger.Error("database ping failed", zap.Error(err))
		http.Error(w, "database connection failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *MetricsServer) UpdateHandler(w http.ResponseWriter, r *http.Request) {
	metricType := chi.URLParam(r, "type")
	metricID := chi.URLParam(r, "name")
	metricValueStr := chi.URLParam(r, "value")

	if metricID == "" {
		http.Error(w, "Metric name is required", http.StatusNotFound)
		return
	}

	if metricType != model.Gauge && metricType != model.Counter {
		http.Error(w, "Invalid metric type", http.StatusBadRequest)
		return
	}

	metric := &model.Metrics{
		ID:    metricID,
		MType: metricType,
	}

	if metricType == model.Gauge {
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

	if err := s.storage.Update(r.Context(), metric); err != nil {
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

	if err := s.storage.Update(r.Context(), &metric); err != nil {
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

	metric, ok, err := s.storage.GetMetric(r.Context(), metricID, metricType)
	if err != nil {
		s.logger.Error("failed to get metric", zap.Error(err))
		http.Error(w, "failed to get metric", http.StatusInternalServerError)
		return
	}
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

	metric, ok, err := s.storage.GetMetric(r.Context(), req.ID, req.MType)
	if err != nil {
		s.logger.Error("failed to get metric", zap.Error(err))
		http.Error(w, "failed to get metric", http.StatusInternalServerError)
		return
	}
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
	metrics, err := s.storage.GetAllMetrics(r.Context())
	if err != nil {
		s.logger.Error("failed to list metrics", zap.Error(err))
		http.Error(w, "failed to list metrics", http.StatusInternalServerError)
		return
	}

	rows := make([]metricRow, 0, len(metrics))
	for _, m := range metrics {
		var valueStr string
		if m.MType == "gauge" && m.Value != nil {
			valueStr = strconv.FormatFloat(*m.Value, 'f', -1, 64)
		} else if m.MType == "counter" && m.Delta != nil {
			valueStr = strconv.FormatInt(*m.Delta, 10)
		}
		rows = append(rows, metricRow{Type: m.MType, Name: m.ID, Value: valueStr})
	}
	var body bytes.Buffer
	if err := metricsListTemplate.Execute(&body, rows); err != nil {
		s.logger.Error("failed to render metrics list", zap.Error(err))
		http.Error(w, "failed to render metrics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body.Bytes())
}

type metricRow struct {
	Type  string
	Name  string
	Value string
}

var metricsListTemplate = template.Must(template.New("metrics-list").Parse(`<!DOCTYPE html>
<html><head><title>Metrics</title></head><body>
<h1>Metrics</h1>
<table border="1">
<tr><th>Type</th><th>Name</th><th>Value</th></tr>
{{range .}}<tr><td>{{.Type}}</td><td>{{.Name}}</td><td>{{.Value}}</td></tr>
{{end}}</table>
</body></html>
`))

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
