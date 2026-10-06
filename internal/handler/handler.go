package handler

import (
	"net/http"
	"strconv"
	"strings"

	"metrics-alerting/internal/storage"
	"metrics-alerting/internal/model"
)

type MetricsServer struct {
	storage storage.Storage
}

func NewMetricsServer(s storage.Storage) *MetricsServer {
	return &MetricsServer{storage: s}
}

func (s *MetricsServer) UpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/update/")

	path = strings.TrimRight(path, "/")
	
	parts := strings.Split(path, "/")

	if len(parts) != 3 {
		http.Error(w, "Metric name and value are required", http.StatusNotFound)
		return
	}

	metricType := parts[0]
	metricID := parts[1]
	metricValueStr := parts[2]

	if metricID == "" {
		http.Error(w, "Metric name is required", http.StatusNotFound)
		return
	}

	if metricType != "gauge" && metricType != "counter" {
		http.Error(w, "Invalid metric type", http.StatusBadRequest)
		return
	}

	metric := &models.Metrics{
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

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK\n"))
}