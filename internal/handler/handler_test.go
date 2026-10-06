package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"metrics-alerting/internal/storage"
)

func TestUpdateHandler_GaugeSuccess(t *testing.T) {
	stor := storage.NewMemStorage()
	server := NewMetricsServer(stor)

	req := httptest.NewRequest(http.MethodPost, "/update/gauge/TestGauge/123.45", nil)
	w := httptest.NewRecorder()

	server.UpdateHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	m, ok := stor.GetMetric("TestGauge", "gauge")
	if !ok || m.Value == nil || *m.Value != 123.45 {
		t.Errorf("Metric not saved correctly, got %+v", m)
	}
}

func TestUpdateHandler_CounterSuccess(t *testing.T) {
	stor := storage.NewMemStorage()
	server := NewMetricsServer(stor)

	req := httptest.NewRequest(http.MethodPost, "/update/counter/TestCounter/10", nil)
	w := httptest.NewRecorder()

	server.UpdateHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	m, ok := stor.GetMetric("TestCounter", "counter")
	if !ok || m.Delta == nil || *m.Delta != 10 {
		t.Errorf("Metric not saved correctly, got %+v", m)
	}
}

func TestUpdateHandler_WithoutID_TrailingSlash(t *testing.T) {
	stor := storage.NewMemStorage()
	server := NewMetricsServer(stor)

	req := httptest.NewRequest(http.MethodPost, "/update/gauge/", nil)
	w := httptest.NewRecorder()

	server.UpdateHandler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for trailing slash, got %d", w.Code)
	}
}

func TestUpdateHandler_WithoutID_MissingParts(t *testing.T) {
	stor := storage.NewMemStorage()
	server := NewMetricsServer(stor)

	req := httptest.NewRequest(http.MethodPost, "/update/gauge", nil)
	w := httptest.NewRecorder()

	server.UpdateHandler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for missing parts, got %d", w.Code)
	}
}

func TestUpdateHandler_WithoutID_DoubleSlash(t *testing.T) {
	stor := storage.NewMemStorage()
	server := NewMetricsServer(stor)

	req := httptest.NewRequest(http.MethodPost, "/update/gauge//100", nil)
	w := httptest.NewRecorder()

	server.UpdateHandler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for double slash, got %d", w.Code)
	}
}

func TestUpdateHandler_InvalidMetricType(t *testing.T) {
	stor := storage.NewMemStorage()
	server := NewMetricsServer(stor)

	req := httptest.NewRequest(http.MethodPost, "/update/invalid/MyMetric/100", nil)
	w := httptest.NewRecorder()

	server.UpdateHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid type, got %d", w.Code)
	}
}

func TestUpdateHandler_InvalidGaugeValue(t *testing.T) {
	stor := storage.NewMemStorage()
	server := NewMetricsServer(stor)

	req := httptest.NewRequest(http.MethodPost, "/update/gauge/MyMetric/not_a_number", nil)
	w := httptest.NewRecorder()

	server.UpdateHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid gauge value, got %d", w.Code)
	}
}

func TestUpdateHandler_InvalidCounterValue(t *testing.T) {
	stor := storage.NewMemStorage()
	server := NewMetricsServer(stor)

	req := httptest.NewRequest(http.MethodPost, "/update/counter/MyCounter/3.14", nil)
	w := httptest.NewRecorder()

	server.UpdateHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for float counter value, got %d", w.Code)
	}
}

func TestUpdateHandler_WrongMethod(t *testing.T) {
	stor := storage.NewMemStorage()
	server := NewMetricsServer(stor)

	req := httptest.NewRequest(http.MethodGet, "/update/gauge/MyMetric/100", nil)
	w := httptest.NewRecorder()

	server.UpdateHandler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for GET request, got %d", w.Code)
	}
}