package storage

import (
	"fmt"
	"sync"

	"metrics-alerting/internal/model"
)


type Storage interface {
	Update(metric *models.Metrics) error
	GetMetric(id string, mType string) (*models.Metrics, bool)
}

type MemStorage struct {
	mu      sync.RWMutex
	metrics map[string]*models.Metrics
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		metrics: make(map[string]*models.Metrics),
	}
}

func (s *MemStorage) Update(metric *models.Metrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s", metric.MType, metric.ID)

	if existing, ok := s.metrics[key]; ok {
		if metric.MType == "counter" && metric.Delta != nil {
			if existing.Delta == nil {
				existing.Delta = new(int64)
			}
			*existing.Delta += *metric.Delta
		}
		if metric.MType == "gauge" && metric.Value != nil {
			existing.Value = metric.Value
		}
		if metric.Hash != "" {
			existing.Hash = metric.Hash
		}
	} else {
		s.metrics[key] = metric
	}

	return nil
}

func (s *MemStorage) GetMetric(id string, mType string) (*models.Metrics, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", mType, id)
	metric, ok := s.metrics[key]
	return metric, ok
}

func (s *MemStorage) GetAllMetrics() map[string]*models.Metrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*models.Metrics, len(s.metrics))
	for k, v := range s.metrics {
		result[k] = v
	}
	return result
}