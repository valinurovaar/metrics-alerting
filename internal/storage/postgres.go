package storage

import (
	"database/sql"
	"errors"

	"metrics-alerting/internal/model"
)

type PostgresStorage struct {
	database *sql.DB
}

func NewPostgresStorage(database *sql.DB) *PostgresStorage {
	return &PostgresStorage{database: database}
}

func (s *PostgresStorage) Update(metric *model.Metrics) error {
	if err := validateMetric(metric); err != nil {
		return err
	}

	switch metric.MType {
	case model.Gauge:
		if metric.Value == nil {
			return errors.New("gauge value is required")
		}
		_, err := s.database.Exec(`
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
		_, err := s.database.Exec(`
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
