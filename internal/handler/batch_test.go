package handler

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"metrics-alerting/internal/storage"
)

func TestUpdatesBatch(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		stor := storage.NewMemStorage()
		server := setupTestServer(stor)
		body := `[ {"id":"G","type":"gauge","value":1.5}, {"id":"C","type":"counter","delta":2}, {"id":"C","type":"counter","delta":3} ]`
		var buf bytes.Buffer
		if compressed {
			gz := gzip.NewWriter(&buf)
			gz.Write([]byte(body))
			gz.Close()
		} else {
			buf.WriteString(body)
		}
		req := httptest.NewRequest(http.MethodPost, "/updates/", &buf)
		if compressed {
			req.Header.Set("Content-Encoding", "gzip")
			req.Header.Set("Accept-Encoding", "gzip")
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status: %d, body: %s", w.Code, w.Body.String())
		}
		metric, ok, err := stor.GetMetric(context.Background(), "C", "counter")
		if err != nil || !ok || *metric.Delta != 5 {
			t.Fatalf("unexpected counter: %+v, %v", metric, err)
		}
		metric, ok, err = stor.GetMetric(context.Background(), "G", "gauge")
		if err != nil || !ok || *metric.Value != 1.5 {
			t.Fatalf("unexpected gauge: %+v, %v", metric, err)
		}
	}
}

func TestUpdatesInvalidBatchIsNotApplied(t *testing.T) {
	for _, body := range []string{
		`{}`, `broken`,
		`[{"id":"C","type":"counter","delta":1},{"id":"G","type":"gauge"}]`,
		`[{"id":"C","type":"counter","delta":1}] {}`,
	} {
		stor := storage.NewMemStorage()
		w := httptest.NewRecorder()
		setupTestServer(stor).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(body)))
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status %d", body, w.Code)
		}
		all, _ := stor.GetAllMetrics(context.Background())
		if len(all) != 0 {
			t.Errorf("invalid batch was partially applied: %+v", all)
		}
	}
}
