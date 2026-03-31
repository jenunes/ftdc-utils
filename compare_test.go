package ftdc

import (
	"testing"
	"time"
)

func TestIsCmpMetric_DirectMatch(t *testing.T) {
	t.Parallel()
	if !isCmpMetric("start") {
		t.Error("expected 'start' to match")
	}
	if !isCmpMetric("end") {
		t.Error("expected 'end' to match")
	}
}

func TestIsCmpMetric_PrefixMatch(t *testing.T) {
	t.Parallel()
	if !isCmpMetric("serverStatus.opcounters.insert") {
		t.Error("expected serverStatus.opcounters.insert to match via prefix")
	}
	if !isCmpMetric("serverStatus.wiredTiger.cache.bytes read into cache") {
		t.Error("expected wiredTiger.cache subkey to match")
	}
}

func TestIsCmpMetric_ModernPaths(t *testing.T) {
	t.Parallel()
	paths := []string{
		"serverStatus.flowControl.isLagged",
		"serverStatus.transactions.totalStarted",
		"serverStatus.tcmalloc.generic.current_allocated_bytes",
		"serverStatus.opLatencies.reads.latency",
		"systemMetrics.cpu.idle_ms",
	}
	for _, p := range paths {
		if !isCmpMetric(p) {
			t.Errorf("expected '%s' to match via modern cmpMetrics", p)
		}
	}
}

func TestIsCmpMetric_NoMatch(t *testing.T) {
	t.Parallel()
	if isCmpMetric("randomPrefix.something") {
		t.Error("expected no match for 'randomPrefix.something'")
	}
}

func TestIsCmpMetric_ObsoletePaths(t *testing.T) {
	t.Parallel()
	obsolete := []string{
		"serverStatus.mem.mapped",
		"serverStatus.mem.mappedWithJournal",
		"serverStatus.metrics.record",
		"serverStatus.writeBacksQueued",
	}
	for _, p := range obsolete {
		if isCmpMetric(p) {
			t.Errorf("obsolete path '%s' should no longer match", p)
		}
	}
}

func TestProximal_Identical(t *testing.T) {
	t.Parallel()
	s := Stats{
		Start:    time.Unix(100, 0),
		End:      time.Unix(200, 0),
		NSamples: 100,
		Metrics: map[string]MetricStat{
			"serverStatus.opcounters.insert": {Avg: 50, Var: 10},
		},
	}
	score, _, ok := Proximal(s, s)
	if !ok {
		t.Errorf("identical stats should pass threshold, score=%f", score)
	}
}

func TestProximal_VeryDifferent(t *testing.T) {
	t.Parallel()
	a := Stats{
		Start:    time.Unix(100, 0),
		End:      time.Unix(200, 0),
		NSamples: 100,
		Metrics: map[string]MetricStat{
			"serverStatus.opcounters.insert": {Avg: 1, Var: 1},
		},
	}
	b := Stats{
		Start:    time.Unix(100, 0),
		End:      time.Unix(200, 0),
		NSamples: 10000,
		Metrics: map[string]MetricStat{
			"serverStatus.opcounters.insert": {Avg: 10000, Var: 10000},
		},
	}
	_, _, ok := Proximal(a, b)
	if ok {
		t.Error("very different stats should fail threshold")
	}
}

func TestCompareMetrics_Equal(t *testing.T) {
	t.Parallel()
	s := Stats{
		Metrics: map[string]MetricStat{
			"x": {Avg: 100, Var: 10},
		},
	}
	score := compareMetrics(s, s, "x")
	if score.Score != 1 {
		t.Errorf("equal metrics should score 1, got %f", score.Score)
	}
	if score.Err != nil {
		t.Errorf("equal metrics should have no error, got %v", score.Err)
	}
}
