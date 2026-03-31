package ftdc

import (
	"testing"
	"time"
)

func TestChunkMap(t *testing.T) {
	t.Parallel()
	c := Chunk{
		Metrics: []Metric{
			{Key: "a", Value: 1},
			{Key: "b", Value: 2},
		},
		NDeltas: 0,
	}
	m := c.Map()
	if m["a"].Value != 1 {
		t.Errorf("expected a=1, got %d", m["a"].Value)
	}
	if m["b"].Value != 2 {
		t.Errorf("expected b=2, got %d", m["b"].Value)
	}
}

func TestChunkExpand_NoDeltas(t *testing.T) {
	t.Parallel()
	c := Chunk{
		Metrics: []Metric{
			{Key: "x", Value: 100, Deltas: []int64{}},
		},
		NDeltas: 0,
	}
	expanded := c.Expand(nil)
	if len(expanded) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(expanded))
	}
	if expanded[0]["x"] != 100 {
		t.Errorf("expected x=100, got %d", expanded[0]["x"])
	}
}

func TestChunkExpand_WithDeltas(t *testing.T) {
	t.Parallel()
	c := Chunk{
		Metrics: []Metric{
			{Key: "x", Value: 10, Deltas: []int64{1, 2, 3}},
		},
		NDeltas: 3,
	}
	expanded := c.Expand(nil)
	if len(expanded) != 4 {
		t.Fatalf("expected 4 samples, got %d", len(expanded))
	}
	expected := []int64{10, 11, 13, 16}
	for i, want := range expected {
		if expanded[i]["x"] != want {
			t.Errorf("sample %d: expected x=%d, got %d", i, want, expanded[i]["x"])
		}
	}
}

func TestChunkExpand_WithIncludeKeys(t *testing.T) {
	t.Parallel()
	c := Chunk{
		Metrics: []Metric{
			{Key: "a", Value: 1, Deltas: []int64{1}},
			{Key: "b", Value: 2, Deltas: []int64{2}},
		},
		NDeltas: 1,
	}
	include := map[string]bool{"a": true}
	expanded := c.Expand(include)
	if len(expanded) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(expanded))
	}
	if _, ok := expanded[0]["b"]; ok {
		t.Error("key 'b' should not be included")
	}
	if expanded[0]["a"] != 1 {
		t.Errorf("expected a=1, got %d", expanded[0]["a"])
	}
}

func TestChunkExpand_PrefixInclude(t *testing.T) {
	t.Parallel()
	c := Chunk{
		Metrics: []Metric{
			{Key: "serverStatus.mem.resident", Value: 100, Deltas: []int64{}},
			{Key: "serverStatus.mem.virtual", Value: 200, Deltas: []int64{}},
			{Key: "other.metric", Value: 300, Deltas: []int64{}},
		},
		NDeltas: 0,
	}
	include := map[string]bool{"serverStatus.mem": true}
	expanded := c.Expand(include)
	if len(expanded) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(expanded))
	}
	if _, ok := expanded[0]["other.metric"]; ok {
		t.Error("other.metric should not be included")
	}
	if expanded[0]["serverStatus.mem.resident"] != 100 {
		t.Errorf("expected resident=100, got %d", expanded[0]["serverStatus.mem.resident"])
	}
	if expanded[0]["serverStatus.mem.virtual"] != 200 {
		t.Errorf("expected virtual=200, got %d", expanded[0]["serverStatus.mem.virtual"])
	}
}

func TestChunkClip_InsideRange(t *testing.T) {
	t.Parallel()
	// start value in milliseconds: 50_000 ms = 50 seconds
	c := Chunk{
		Metrics: []Metric{
			{Key: "start", Value: 50_000, Deltas: []int64{1000, 1000}},
			{Key: "val", Value: 10, Deltas: []int64{1, 2}},
		},
		NDeltas: 2,
	}
	start := time.Unix(0, 0)
	end := time.Unix(100, 0)
	ok := c.Clip(start, end)
	if !ok {
		t.Error("expected Clip to return true")
	}
}

func TestChunkClip_OutsideRange(t *testing.T) {
	t.Parallel()
	c := Chunk{
		Metrics: []Metric{
			{Key: "start", Value: 200_000, Deltas: []int64{1000}},
		},
		NDeltas: 1,
	}
	start := time.Unix(0, 0)
	end := time.Unix(0, 0)
	ok := c.Clip(start, end)
	if ok {
		t.Error("expected Clip to return false for chunk outside range")
	}
}

func TestChunkClip_MutationPropagates(t *testing.T) {
	t.Parallel()
	c := Chunk{
		Metrics: []Metric{
			{Key: "start", Value: 1000, Deltas: []int64{1000, 1000, 1000, 1000}},
			{Key: "val", Value: 100, Deltas: []int64{10, 20, 30, 40}},
		},
		NDeltas: 4,
	}
	start := time.Unix(0, 0)
	end := time.Unix(100, 0)
	c.Clip(start, end)

	for _, m := range c.Metrics {
		if len(m.Deltas) == 0 && c.NDeltas > 0 {
			t.Errorf("metric %s has no deltas after Clip, but NDeltas=%d", m.Key, c.NDeltas)
		}
	}
}
