package storage

import (
	"testing"

	"metrics-alerting/internal/model"
)

func TestMemStorage_UpdateGauge(t *testing.T) {
	stor := NewMemStorage()

	value := 42.5
	err := stor.Update(&models.Metrics{
		ID:    "TestGauge",
		MType: "gauge",
		Value: &value,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	m, ok := stor.GetMetric("TestGauge", "gauge")
	if !ok {
		t.Fatal("Metric not found")
	}
	if m.Value == nil || *m.Value != 42.5 {
		t.Errorf("Expected gauge value 42.5, got %+v", m.Value)
	}
}

func TestMemStorage_UpdateGaugeOverwrite(t *testing.T) {
	stor := NewMemStorage()

	v1 := 10.0
	v2 := 20.0
	stor.Update(&models.Metrics{ID: "G", MType: "gauge", Value: &v1})
	stor.Update(&models.Metrics{ID: "G", MType: "gauge", Value: &v2})

	m, _ := stor.GetMetric("G", "gauge")
	if *m.Value != 20.0 {
		t.Errorf("Expected gauge to be overwritten to 20.0, got %f", *m.Value)
	}
}

func TestMemStorage_UpdateCounter(t *testing.T) {
	stor := NewMemStorage()

	delta := int64(10)
	err := stor.Update(&models.Metrics{
		ID:    "TestCounter",
		MType: "counter",
		Delta: &delta,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	m, ok := stor.GetMetric("TestCounter", "counter")
	if !ok {
		t.Fatal("Metric not found")
	}
	if m.Delta == nil || *m.Delta != 10 {
		t.Errorf("Expected counter delta 10, got %+v", m.Delta)
	}
}

func TestMemStorage_UpdateCounterAccumulate(t *testing.T) {
	stor := NewMemStorage()

	d1 := int64(5)
	d2 := int64(3)
	stor.Update(&models.Metrics{ID: "C", MType: "counter", Delta: &d1})
	stor.Update(&models.Metrics{ID: "C", MType: "counter", Delta: &d2})

	m, _ := stor.GetMetric("C", "counter")
	if *m.Delta != 8 {
		t.Errorf("Expected accumulated counter 8, got %d", *m.Delta)
	}
}
