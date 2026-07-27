package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"metrics-alerting/internal/model"
	"metrics-alerting/internal/storage"
)

func setupTestServer(stor storage.Storage) http.Handler {
	return NewMetricsServer(stor).Routes()
}

func TestUpdateHandler_GaugeSuccess(t *testing.T) {
	stor := storage.NewMemStorage()
	server := setupTestServer(stor)

	req := httptest.NewRequest(http.MethodPost, "/update/gauge/TestGauge/123.45", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

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
	server := setupTestServer(stor)

	req := httptest.NewRequest(http.MethodPost, "/update/counter/TestCounter/10", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	m, ok := stor.GetMetric("TestCounter", "counter")
	if !ok || m.Delta == nil || *m.Delta != 10 {
		t.Errorf("Metric not saved correctly, got %+v", m)
	}
}

func TestUpdateHandler_WithoutID_DoubleSlash(t *testing.T) {
	stor := storage.NewMemStorage()
	server := setupTestServer(stor)

	req := httptest.NewRequest(http.MethodPost, "/update/gauge//100", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for empty name, got %d", w.Code)
	}
}

func TestUpdateHandler_InvalidMetricType(t *testing.T) {
	stor := storage.NewMemStorage()
	server := setupTestServer(stor)

	req := httptest.NewRequest(http.MethodPost, "/update/invalid/MyMetric/100", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid type, got %d", w.Code)
	}
}

func TestUpdateHandler_InvalidGaugeValue(t *testing.T) {
	stor := storage.NewMemStorage()
	server := setupTestServer(stor)

	req := httptest.NewRequest(http.MethodPost, "/update/gauge/MyMetric/not_a_number", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid gauge value, got %d", w.Code)
	}
}

func TestUpdateHandler_InvalidCounterValue(t *testing.T) {
	stor := storage.NewMemStorage()
	server := setupTestServer(stor)

	req := httptest.NewRequest(http.MethodPost, "/update/counter/MyCounter/3.14", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for float counter value, got %d", w.Code)
	}
}

func TestGetValueHandler_GaugeSuccess(t *testing.T) {
	stor := storage.NewMemStorage()
	server := setupTestServer(stor)

	value := 42.5
	stor.Update(&models.Metrics{ID: "TestGauge", MType: "gauge", Value: &value})

	req := httptest.NewRequest(http.MethodGet, "/value/gauge/TestGauge", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body, _ := io.ReadAll(w.Body)
	if string(body) != "42.5" {
		t.Errorf("Expected body 42.5, got %q", body)
	}
}

func TestGetValueHandler_CounterSuccess(t *testing.T) {
	stor := storage.NewMemStorage()
	server := setupTestServer(stor)

	delta := int64(100)
	stor.Update(&models.Metrics{ID: "TestCounter", MType: "counter", Delta: &delta})

	req := httptest.NewRequest(http.MethodGet, "/value/counter/TestCounter", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body, _ := io.ReadAll(w.Body)
	if string(body) != "100" {
		t.Errorf("Expected body 100, got %q", body)
	}
}

func TestGetValueHandler_NotFound(t *testing.T) {
	stor := storage.NewMemStorage()
	server := setupTestServer(stor)

	req := httptest.NewRequest(http.MethodGet, "/value/gauge/Unknown", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestGetValueHandler_InvalidType(t *testing.T) {
	stor := storage.NewMemStorage()
	server := setupTestServer(stor)

	req := httptest.NewRequest(http.MethodGet, "/value/invalid/MyMetric", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for invalid type, got %d", w.Code)
	}
}

func TestListHandler_Success(t *testing.T) {
	stor := storage.NewMemStorage()
	server := setupTestServer(stor)

	gaugeVal := 10.5
	counterDelta := int64(7)
	stor.Update(&models.Metrics{ID: "MyGauge", MType: "gauge", Value: &gaugeVal})
	stor.Update(&models.Metrics{ID: "MyCounter", MType: "counter", Delta: &counterDelta})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body, _ := io.ReadAll(w.Body)
	html := string(body)

	if !strings.Contains(html, "MyGauge") || !strings.Contains(html, "10.5") {
		t.Errorf("Expected HTML to contain gauge metric, got %q", html)
	}
	if !strings.Contains(html, "MyCounter") || !strings.Contains(html, "7") {
		t.Errorf("Expected HTML to contain counter metric, got %q", html)
	}
}
