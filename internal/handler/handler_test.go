package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"metrics-alerting/internal/model"
	"metrics-alerting/internal/storage"
)

type databaseStub struct {
	err error
}

func (d databaseStub) PingContext(context.Context) error {
	return d.err
}

func setupTestServer(stor storage.Storage) http.Handler {
	logger, _ := zap.NewDevelopment()
	return NewMetricsServer(stor, logger).Routes()
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

	m, ok, err := stor.GetMetric(context.Background(), "TestGauge", "gauge")
	if err != nil {
		t.Fatalf("GetMetric failed: %v", err)
	}
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

	m, ok, err := stor.GetMetric(context.Background(), "TestCounter", "counter")
	if err != nil {
		t.Fatalf("GetMetric failed: %v", err)
	}
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
	stor.Update(context.Background(), &model.Metrics{ID: "TestGauge", MType: "gauge", Value: &value})

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
	stor.Update(context.Background(), &model.Metrics{ID: "TestCounter", MType: "counter", Delta: &delta})

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
	stor.Update(context.Background(), &model.Metrics{ID: "MyGauge", MType: "gauge", Value: &gaugeVal})
	stor.Update(context.Background(), &model.Metrics{ID: "MyCounter", MType: "counter", Delta: &counterDelta})

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

func TestListHandler_EscapesMetricName(t *testing.T) {
	stor := storage.NewMemStorage()
	value := 1.0
	maliciousName := `<script>alert("xss")</script>`
	if err := stor.Update(context.Background(), &model.Metrics{
		ID: maliciousName, MType: model.Gauge, Value: &value,
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	setupTestServer(stor).ServeHTTP(w, req)
	body := w.Body.String()

	if strings.Contains(body, maliciousName) {
		t.Fatalf("response contains unescaped metric name: %q", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("response does not contain escaped metric name: %q", body)
	}
}

func TestPingHandler(t *testing.T) {
	tests := []struct {
		name       string
		database   []DatabasePinger
		wantStatus int
	}{
		{
			name:       "database is available",
			database:   []DatabasePinger{databaseStub{}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "database ping fails",
			database:   []DatabasePinger{databaseStub{err: errors.New("connection failed")}},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "database is not configured",
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stor := storage.NewMemStorage()
			logger := zap.NewNop()
			server := NewMetricsServer(stor, logger, tt.database...).Routes()

			req := httptest.NewRequest(http.MethodGet, "/ping", nil)
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}
