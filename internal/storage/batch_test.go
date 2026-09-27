package storage

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"metrics-alerting/internal/model"
)

func TestMemBatchConcurrent(t *testing.T) {
	stor := NewMemStorage()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				delta := int64(1)
				if err := stor.UpdateBatch(context.Background(), []model.Metrics{{ID: "C", MType: model.Counter, Delta: &delta}}); err != nil {
					t.Error(err)
				}
				delta = 100 // Input must not alias stored values.
				stor.GetAllMetrics(context.Background())
			}
		}()
	}
	wg.Wait()
	m, _, _ := stor.GetMetric(context.Background(), "C", model.Counter)
	if *m.Delta != 1000 {
		t.Fatalf("counter = %d", *m.Delta)
	}
}

func TestPostgresBatchTransaction(t *testing.T) {
	for _, fail := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		stor := NewPostgresStorage(db)
		delta := int64(2)
		mock.ExpectBegin()
		first := mock.ExpectExec("INSERT INTO metrics").WithArgs("C", model.Counter, delta)
		if fail {
			first.WillReturnError(errors.New("write failed"))
			mock.ExpectRollback()
		} else {
			first.WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("INSERT INTO metrics").WithArgs("C", model.Counter, delta).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()
		}
		batch := []model.Metrics{{ID: "C", MType: model.Counter, Delta: &delta}, {ID: "C", MType: model.Counter, Delta: &delta}}
		if err := stor.UpdateBatch(context.Background(), batch); (err != nil) != fail {
			t.Errorf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		db.Close()
	}
}
