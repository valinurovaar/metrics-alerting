package storage

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"metrics-alerting/internal/model"
	"metrics-alerting/internal/retry"
)

func TestBatchRetryRollsBackAndRestarts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewPostgresStorage(db)
	s.retryPolicy = retry.Policy{Wait: func(context.Context, time.Duration) error { return nil }}
	delta := int64(1)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO metrics").WithArgs("A", model.Counter, delta).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO metrics").WithArgs("B", model.Counter, delta).WillReturnError(&pq.Error{Code: "08006"})
	mock.ExpectRollback()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO metrics").WithArgs("A", model.Counter, delta).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO metrics").WithArgs("B", model.Counter, delta).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	err = s.UpdateBatch([]model.Metrics{{ID: "B", MType: model.Counter, Delta: &delta}, {ID: "A", MType: model.Counter, Delta: &delta}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresPermanentError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewPostgresStorage(db)
	value := 1.0
	mock.ExpectExec("INSERT INTO metrics").WillReturnError(&pq.Error{Code: "23505"})
	if err := s.Update(&model.Metrics{ID: "G", MType: model.Gauge, Value: &value}); err == nil {
		t.Fatal("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresReadRetry(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewPostgresStorage(db)
	s.retryPolicy = retry.Policy{Wait: func(context.Context, time.Duration) error { return nil }}
	mock.ExpectQuery("SELECT gauge_value, counter_value").WillReturnError(&pq.Error{Code: "08003"})
	mock.ExpectQuery("SELECT gauge_value, counter_value").WillReturnRows(sqlmock.NewRows([]string{"gauge_value", "counter_value"}).AddRow(1.5, nil))
	m, found, err := s.GetMetric("G", model.Gauge)
	if err != nil || !found || *m.Value != 1.5 {
		t.Fatal(m, found, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBatchDoesNotReplayUnknownCommit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewPostgresStorage(db)
	delta := int64(1)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO metrics").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(&pq.Error{Code: "08007"})
	if err := s.UpdateBatch([]model.Metrics{{ID: "C", MType: model.Counter, Delta: &delta}}); err == nil {
		t.Fatal("expected commit error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
