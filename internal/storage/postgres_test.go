package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"metrics-alerting/internal/model"
)

func TestPostgresStorageUpdate(t *testing.T) {
	tests := []struct {
		name   string
		metric *model.Metrics
		query  string
		args   []driver.Value
	}{
		{
			name: "gauge is replaced",
			metric: func() *model.Metrics {
				value := 12.5
				return &model.Metrics{ID: "temperature", MType: model.Gauge, Value: &value}
			}(),
			query: `INSERT INTO metrics .* gauge_value = EXCLUDED\.gauge_value`,
			args:  []driver.Value{"temperature", model.Gauge, 12.5},
		},
		{
			name: "counter is accumulated",
			metric: func() *model.Metrics {
				delta := int64(3)
				return &model.Metrics{ID: "requests", MType: model.Counter, Delta: &delta}
			}(),
			query: `INSERT INTO metrics .* counter_value = metrics\.counter_value \+ EXCLUDED\.counter_value`,
			args:  []driver.Value{"requests", model.Counter, int64(3)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("create mock database: %v", err)
			}
			defer database.Close()

			expectation := mock.ExpectExec(tt.query)
			expectation.WithArgs(tt.args...).WillReturnResult(sqlmock.NewResult(1, 1))

			stor := NewPostgresStorage(database)
			if err := stor.Update(context.Background(), tt.metric); err != nil {
				t.Fatalf("Update failed: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet SQL expectations: %v", err)
			}
		})
	}
}

func TestPostgresStorageGetMetric(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock database: %v", err)
	}
	defer database.Close()

	mock.ExpectQuery(`SELECT gauge_value, counter_value FROM metrics`).
		WithArgs("temperature", model.Gauge).
		WillReturnRows(sqlmock.NewRows([]string{"gauge_value", "counter_value"}).AddRow(12.5, nil))

	stor := NewPostgresStorage(database)
	metric, ok, err := stor.GetMetric(context.Background(), "temperature", model.Gauge)
	if err != nil {
		t.Fatalf("GetMetric failed: %v", err)
	}
	if !ok || metric.Value == nil || *metric.Value != 12.5 {
		t.Fatalf("unexpected metric: %+v, found: %t", metric, ok)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet SQL expectations: %v", err)
	}
}

func TestPostgresStorageGetMetricNotFound(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock database: %v", err)
	}
	defer database.Close()

	mock.ExpectQuery(`SELECT gauge_value, counter_value FROM metrics`).
		WithArgs("unknown", model.Counter).
		WillReturnError(sql.ErrNoRows)

	stor := NewPostgresStorage(database)
	metric, ok, err := stor.GetMetric(context.Background(), "unknown", model.Counter)
	if err != nil {
		t.Fatalf("GetMetric failed: %v", err)
	}
	if ok || metric != nil {
		t.Fatalf("expected no metric, got %+v", metric)
	}
}
