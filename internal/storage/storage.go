package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"metrics-alerting/internal/model"
)

type Storage interface {
	Update(metric *model.Metrics) error
	GetMetric(id string, mType string) (*model.Metrics, bool)
	GetAllMetrics() map[string]*model.Metrics
}

type PersistenceConfig struct {
	FilePath string

	Restore bool

	StoreInterval time.Duration
}

type metricDTO struct {
	ID    string   `json:"id"`
	MType string   `json:"type"`
	Value *float64 `json:"value,omitempty"`
	Delta *int64   `json:"delta,omitempty"`
}

type MemStorage struct {
	mu      sync.RWMutex
	saveMu  sync.Mutex
	metrics map[string]*model.Metrics

	path     string
	interval time.Duration

	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		metrics: make(map[string]*model.Metrics),
	}
}

func NewPersistentMemStorage(cfg PersistenceConfig) (*MemStorage, error) {
	if cfg.FilePath == "" {
		cfg.FilePath = "metrics.json"
	}

	if cfg.StoreInterval < 0 {
		cfg.StoreInterval = 0
	}

	s := &MemStorage{
		metrics:  make(map[string]*model.Metrics),
		path:     cfg.FilePath,
		interval: cfg.StoreInterval,
	}

	if cfg.Restore {
		if err := s.load(); err != nil {
			return nil, fmt.Errorf("load metrics from %s: %w", cfg.FilePath, err)
		}
	}

	if cfg.StoreInterval > 0 {
		s.stop = make(chan struct{})
		s.wg.Add(1)
		go s.autoSave(cfg.StoreInterval)
	}

	return s, nil
}

func (s *MemStorage) Update(metric *model.Metrics) error {
	if metric == nil {
		return errors.New("metric is nil")
	}

	if metric.ID == "" {
		return errors.New("metric id is required")
	}

	if metric.MType != "gauge" && metric.MType != "counter" {
		return errors.New("invalid metric type")
	}

	key := metricKey(metric.MType, metric.ID)

	s.mu.Lock()

	existing, ok := s.metrics[key]
	if !ok {
		cp := copyMetric(metric)

		if cp.MType == "gauge" {
			cp.Delta = nil
		} else {
			cp.Value = nil
		}

		s.metrics[key] = cp
	} else {
		switch metric.MType {
		case "gauge":
			if metric.Value != nil {
				v := *metric.Value
				existing.Value = &v
				existing.Delta = nil
			}

		case "counter":
			if metric.Delta != nil {
				var newDelta int64

				if existing.Delta != nil {
					newDelta = *existing.Delta + *metric.Delta
				} else {
					newDelta = *metric.Delta
				}

				existing.Delta = &newDelta
				existing.Value = nil
			}
		}
	}

	s.mu.Unlock()

	if s.path != "" && s.interval == 0 {
		return s.Save()
	}

	return nil
}

func (s *MemStorage) GetMetric(id string, mType string) (*model.Metrics, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := metricKey(mType, id)

	metric, ok := s.metrics[key]
	if !ok {
		return nil, false
	}

	return copyMetric(metric), true
}

func (s *MemStorage) GetAllMetrics() map[string]*model.Metrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*model.Metrics, len(s.metrics))

	for k, v := range s.metrics {
		result[k] = copyMetric(v)
	}

	return result
}

func (s *MemStorage) Save() error {
	if s.path == "" {
		return nil
	}

	s.mu.RLock()

	dtos := make([]metricDTO, 0, len(s.metrics))

	for _, m := range s.metrics {
		if m == nil {
			continue
		}

		dto := metricDTO{
			ID:    m.ID,
			MType: m.MType,
		}

		if m.Value != nil {
			v := *m.Value
			dto.Value = &v
		}

		if m.Delta != nil {
			d := *m.Delta
			dto.Delta = &d
		}

		dtos = append(dtos, dto)
	}

	s.mu.RUnlock()

	sort.Slice(dtos, func(i, j int) bool {
		if dtos[i].MType != dtos[j].MType {
			return dtos[i].MType < dtos[j].MType
		}

		return dtos[i].ID < dtos[j].ID
	})

	data, err := json.MarshalIndent(dtos, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}

	data = append(data, '\n')

	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	dir := filepath.Dir(s.path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}

	tmpName := tmp.Name()

	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}

	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replace storage file: %w", err)
	}

	return nil
}

func (s *MemStorage) Close() {
	if s.stop != nil {
		s.stopOnce.Do(func() {
			close(s.stop)
		})

		s.wg.Wait()
	}

	if s.path != "" {
		_ = s.Save()
	}
}

func (s *MemStorage) load() error {
	if s.path == "" {
		return nil
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}

	var dtos []metricDTO
	if err := json.Unmarshal(data, &dtos); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, dto := range dtos {
		if dto.ID == "" {
			continue
		}

		if dto.MType != "gauge" && dto.MType != "counter" {
			continue
		}

		m := &model.Metrics{
			ID:    dto.ID,
			MType: dto.MType,
		}

		switch m.MType {
		case "gauge":
			if dto.Value == nil {
				continue
			}

			v := *dto.Value
			m.Value = &v
			m.Delta = nil

		case "counter":
			if dto.Delta == nil {
				continue
			}

			d := *dto.Delta
			m.Delta = &d
			m.Value = nil
		}

		s.metrics[metricKey(m.MType, m.ID)] = m
	}

	return nil
}

func (s *MemStorage) autoSave(interval time.Duration) {
	defer s.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = s.Save()
		case <-s.stop:
			return
		}
	}
}

func metricKey(mType, id string) string {
	return fmt.Sprintf("%s:%s", mType, id)
}

func copyMetric(m *model.Metrics) *model.Metrics {
	if m == nil {
		return nil
	}

	cp := *m

	if m.Value != nil {
		v := *m.Value
		cp.Value = &v
	}

	if m.Delta != nil {
		d := *m.Delta
		cp.Delta = &d
	}

	return &cp
}