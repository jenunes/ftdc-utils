package ftdc

import (
	"testing"
	"time"
)

func TestComputeMetricStat_Basic(t *testing.T) {
	t.Parallel()
	m := Metric{Key: "test", Value: 100, Deltas: []int64{10, 10, 10}}
	stat := computeMetricStat(m)
	if stat.Avg != 10 {
		t.Errorf("expected avg=10, got %d", stat.Avg)
	}
	if stat.Var != 0 {
		t.Errorf("expected var=0 (constant deltas), got %d", stat.Var)
	}
}

func TestComputeMetricStat_Varying(t *testing.T) {
	t.Parallel()
	m := Metric{Key: "test", Value: 0, Deltas: []int64{2, 4, 6, 8}}
	stat := computeMetricStat(m)
	// avg = (2+4+6+8)/4 = 5
	if stat.Avg != 5 {
		t.Errorf("expected avg=5, got %d", stat.Avg)
	}
	// var = ((2-5)^2 + (4-5)^2 + (6-5)^2 + (8-5)^2) / 4 = (9+1+1+9)/4 = 5
	if stat.Var != 5 {
		t.Errorf("expected var=5, got %d", stat.Var)
	}
}

func TestComputeMetricStat_Empty(t *testing.T) {
	t.Parallel()
	m := Metric{Key: "test", Value: 100, Deltas: []int64{}}
	stat := computeMetricStat(m)
	if stat.Avg != -1 || stat.Var != -1 {
		t.Errorf("expected avg=-1 var=-1 for empty deltas, got avg=%d var=%d", stat.Avg, stat.Var)
	}
}

func TestChunkStats(t *testing.T) {
	t.Parallel()
	c := Chunk{
		Metrics: []Metric{
			{Key: "start", Value: 1000_000, Deltas: []int64{1000, 1000}},
			{Key: "val", Value: 50, Deltas: []int64{5, 10}},
		},
		NDeltas: 2,
	}
	s := c.Stats()
	if s.NSamples != 3 {
		t.Errorf("expected 3 samples, got %d", s.NSamples)
	}
	if _, ok := s.Metrics["val"]; !ok {
		t.Error("expected 'val' in metrics")
	}
}

func TestMergeStats_Empty(t *testing.T) {
	t.Parallel()
	m := MergeStats()
	if m.Metrics == nil {
		t.Error("expected non-nil metrics map")
	}
	if m.NSamples != 0 {
		t.Errorf("expected 0 samples, got %d", m.NSamples)
	}
}

func TestMergeStats_Single(t *testing.T) {
	t.Parallel()
	s := Stats{
		Start:    time.Unix(100, 0),
		End:      time.Unix(200, 0),
		NSamples: 10,
		Metrics: map[string]MetricStat{
			"x": {Avg: 5, Var: 2},
		},
	}
	m := MergeStats(s)
	if m.NSamples != 10 {
		t.Errorf("expected 10, got %d", m.NSamples)
	}
	if m.Metrics["x"].Avg != 5 {
		t.Errorf("expected avg=5, got %d", m.Metrics["x"].Avg)
	}
}

func TestMergeStats_Multiple(t *testing.T) {
	t.Parallel()
	s1 := Stats{
		Start:    time.Unix(100, 0),
		End:      time.Unix(200, 0),
		NSamples: 10,
		Metrics: map[string]MetricStat{
			"x": {Avg: 10, Var: 1},
		},
	}
	s2 := Stats{
		Start:    time.Unix(200, 0),
		End:      time.Unix(300, 0),
		NSamples: 20,
		Metrics: map[string]MetricStat{
			"x": {Avg: 20, Var: 4},
		},
	}
	m := MergeStats(s1, s2)
	if m.NSamples != 30 {
		t.Errorf("expected 30, got %d", m.NSamples)
	}
	if m.Start != time.Unix(100, 0) {
		t.Errorf("expected start=100, got %v", m.Start)
	}
	if m.End != time.Unix(300, 0) {
		t.Errorf("expected end=300, got %v", m.End)
	}
}

func TestWeightedAvg_ZeroWeight(t *testing.T) {
	t.Parallel()
	result := weightedAvg([]int64{10, 20}, []int64{0, 0})
	if result != 0 {
		t.Errorf("expected 0 for zero total weight, got %d", result)
	}
}

func TestWeightedVar_ZeroWeight(t *testing.T) {
	t.Parallel()
	result := weightedVar(10, []int64{10, 20}, []int64{1, 2}, []int64{0, 0})
	if result != 0 {
		t.Errorf("expected 0 for zero total weight, got %d", result)
	}
}
