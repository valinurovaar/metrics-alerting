package storage

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"metrics-alerting/internal/model"
	"metrics-alerting/internal/retry"
)

type PostgresStorage struct {
	database    *sql.DB
	retryPolicy retry.Policy
}

func NewPostgresStorage(database *sql.DB) *PostgresStorage {
	return &PostgresStorage{database: database}
}

func (s *PostgresStorage) Update(metric *model.Metrics) error {
	if err := validateMetric(metric); err != nil {
		return err
	}
	return s.retryPolicy.Do(context.Background(), func() error {
		return updatePostgres(s.database, metric)
	}, retry.IsPostgresConnectionError)
}

func (s *PostgresStorage) UpdateBatch(metrics []model.Metrics) error {
	for i := range metrics {
		if err := validateMetric(&metrics[i]); err != nil {
			return err
		}
	}
	if len(metrics) == 0 {
		return nil
	}
	// Acquire row locks in a consistent order, preserving duplicate metric order.
	ordered := append([]model.Metrics(nil), metrics...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return metricKey(ordered[i].MType, ordered[i].ID) < metricKey(ordered[j].MType, ordered[j].ID)
	})
	return s.retryPolicy.Do(context.Background(), func() error {
		return s.updateBatchOnce(ordered)
	}, func(err error) bool {
		var commit *commitError
		return !errors.As(err, &commit) && retry.IsPostgresConnectionError(err)
	})
}

// A failed COMMIT can have an unknown outcome. Do not replay counter increments.
type commitError struct{ error }

func (e *commitError) Unwrap() error { return e.error }

func (s *PostgresStorage) updateBatchOnce(ordered []model.Metrics) error {
	tx, err := s.database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i := range ordered {
		if err := updatePostgres(tx, &ordered[i]); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return &commitError{err}
	}
	return nil
}

type sqlExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func updatePostgres(executor sqlExecutor, metric *model.Metrics) error {

	switch metric.MType {
	case model.Gauge:
		if metric.Value == nil {
			return errors.New("gauge value is required")
		}
		_, err := executor.Exec(`
			INSERT INTO metrics (id, type, gauge_value, counter_value)
			VALUES ($1, $2, $3, NULL)
			ON CONFLICT (id, type) DO UPDATE
			SET gauge_value = EXCLUDED.gauge_value, counter_value = NULL
		`, metric.ID, metric.MType, *metric.Value)
		return err

	case model.Counter:
		if metric.Delta == nil {
			return errors.New("counter delta is required")
		}
		_, err := executor.Exec(`
			INSERT INTO metrics (id, type, gauge_value, counter_value)
			VALUES ($1, $2, NULL, $3)
			ON CONFLICT (id, type) DO UPDATE
			SET counter_value = metrics.counter_value + EXCLUDED.counter_value,
				gauge_value = NULL
		`, metric.ID, metric.MType, *metric.Delta)
		return err
	}

	return errors.New("invalid metric type")
}

func (s *PostgresStorage) GetMetric(id string, mType string) (*model.Metrics, bool, error) {
	var metric *model.Metrics
	var found bool
	err := s.retryPolicy.Do(context.Background(), func() error {
		var err error
		metric, found, err = s.getMetricOnce(id, mType)
		return err
	}, retry.IsPostgresConnectionError)
	return metric, found, err
}

func (s *PostgresStorage) getMetricOnce(id string, mType string) (*model.Metrics, bool, error) {
	metric := &model.Metrics{ID: id, MType: mType}
	var value sql.NullFloat64
	var delta sql.NullInt64

	err := s.database.QueryRow(`
		SELECT gauge_value, counter_value
		FROM metrics
		WHERE id = $1 AND type = $2
	`, id, mType).Scan(&value, &delta)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	if value.Valid {
		metric.Value = &value.Float64
	}
	if delta.Valid {
		metric.Delta = &delta.Int64
	}

	return metric, true, nil
}

func (s *PostgresStorage) GetAllMetrics() (map[string]*model.Metrics, error) {
	var metrics map[string]*model.Metrics
	err := s.retryPolicy.Do(context.Background(), func() error {
		var err error
		metrics, err = s.getAllMetricsOnce()
		return err
	}, retry.IsPostgresConnectionError)
	return metrics, err
}

func (s *PostgresStorage) getAllMetricsOnce() (map[string]*model.Metrics, error) {
	result := make(map[string]*model.Metrics)
	rows, err := s.database.Query(`
		SELECT id, type, gauge_value, counter_value
		FROM metrics
		ORDER BY type, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		metric := &model.Metrics{}
		var value sql.NullFloat64
		var delta sql.NullInt64
		if err := rows.Scan(&metric.ID, &metric.MType, &value, &delta); err != nil {
			return nil, err
		}

		if value.Valid {
			metric.Value = &value.Float64
		}
		if delta.Valid {
			metric.Delta = &delta.Int64
		}
		result[metricKey(metric.MType, metric.ID)] = metric
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
