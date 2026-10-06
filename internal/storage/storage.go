package storage

import (
	"fmt"
	"sync"

	"metrics-alerting/internal/model"
)

type Storage interface {
	Update(metric *model.Metrics) error
	GetMetric(id string, mType string) (*model.Metrics, bool)
	GetAllMetrics() map[string]*model.Metrics
}

type MemStorage struct {
	mu      sync.RWMutex
	metrics map[string]*model.Metrics
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		metrics: make(map[string]*model.Metrics),
	}
}

func (s *MemStorage) Update(metric *model.Metrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s", metric.MType, metric.ID)

	if existing, ok := s.metrics[key]; ok {
		if metric.MType == model.Counter && metric.Delta != nil {
			if existing.Delta == nil {
				existing.Delta = new(int64)
			}
			*existing.Delta += *metric.Delta
		}
		if metric.MType == model.Gauge && metric.Value != nil {
			existing.Value = metric.Value
		}
	} else {
		s.metrics[key] = metric
	}

	return nil
}

func (s *MemStorage) GetMetric(id string, mType string) (*model.Metrics, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", mType, id)
	metric, ok := s.metrics[key]
	return metric, ok
}

func (s *MemStorage) GetAllMetrics() map[string]*model.Metrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*model.Metrics, len(s.metrics))
	for k, v := range s.metrics {
		result[k] = v
	}
	return result
}
