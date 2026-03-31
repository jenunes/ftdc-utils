package ftdc

import (
	"io"
	"math"
	"sync"
	"time"
)

// MetricStat represents basic statistics for a single metric
type MetricStat struct {
	// Avg is the mean of the metric's deltas (first derivative).
	Avg int64

	// Var is the variance (related to the absolute second derivative).
	Var int64
}

// Stats represents basic statistics for a set of metric samples.
type Stats struct {
	Start    time.Time
	End      time.Time
	Metrics  map[string]MetricStat
	NSamples int
}

// Stats produces Stats for the Chunk
func (c *Chunk) Stats() (s Stats) {
	s.NSamples = 1 + c.NDeltas
	s.Metrics = make(map[string]MetricStat, len(c.Metrics))
	var start, end int64
	for _, m := range c.Metrics {
		s.Metrics[m.Key] = computeMetricStat(m)
		if m.Key == "start" {
			start = m.Value / 1000
			end = (m.Value + sum(m.Deltas...)) / 1000
		}
	}
	s.Start = time.Unix(start, 0)
	s.End = time.Unix(end, 0)
	return
}

// ComputeStats takes an FTDC diagnostic file in the form of an
// io.Reader, and computes statistics for all metrics on each chunk.
func ComputeStats(r io.Reader) (cs []Stats, err error) {
	ch := make(chan Chunk)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		for c := range ch {
			cs = append(cs, c.Stats())
		}
		wg.Done()
	}()
	err = Chunks(r, ch)
	if err != nil {
		return
	}
	wg.Wait()
	return
}

// ComputeStatsInterval takes an FTDC diagnostic file in the form of an
// io.Reader, and computes statistics for all metrics within the given time
// frame, clipping chunks to fit.
func ComputeStatsInterval(r io.Reader, start, end time.Time) (cs []Stats, err error) {
	ch := make(chan Chunk)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		for c := range ch {
			if c.Clip(start, end) {
				cs = append(cs, c.Stats())
			}
		}
		wg.Done()
	}()
	err = Chunks(r, ch)
	if err != nil {
		return
	}
	wg.Wait()
	return
}

// MergeStats computes a time-weighted merge of Stats.
func MergeStats(cs ...Stats) (m Stats) {
	if len(cs) == 0 {
		m.Metrics = make(map[string]MetricStat)
		return
	}

	var start int64 = math.MaxInt64
	var end int64 = math.MinInt64
	weights := make([]int64, len(cs))
	avgs := make(map[string][]int64)
	vars := make(map[string][]int64)
	for i, s := range cs {
		m.NSamples += s.NSamples
		sStart := s.Start.Unix()
		sEnd := s.End.Unix()
		if sStart < start {
			start = sStart
		}
		if sEnd > end {
			end = sEnd
		}
		weights[i] = sEnd - sStart
		for k, v := range s.Metrics {
			if _, ok := avgs[k]; !ok {
				avgs[k] = make([]int64, len(cs))
				vars[k] = make([]int64, len(cs))
			}
			avgs[k][i] = v.Avg
			vars[k][i] = v.Var
		}
	}
	m.Start = time.Unix(start, 0)
	m.End = time.Unix(end, 0)
	m.Metrics = make(map[string]MetricStat, len(avgs))
	for k := range avgs {
		avg := weightedAvg(avgs[k], weights)
		variance := weightedVar(avg, avgs[k], vars[k], weights)
		m.Metrics[k] = MetricStat{
			Avg: avg,
			Var: variance,
		}
	}
	return
}

func computeMetricStat(m Metric) MetricStat {
	if len(m.Deltas) == 0 {
		return MetricStat{-1, -1}
	}
	l := make([]int64, len(m.Deltas))
	copy(l, m.Deltas)
	avg := sum(l...) / int64(len(l))
	var variance int64
	for _, x := range l {
		variance += square(x - avg)
	}
	variance /= int64(len(l))
	return MetricStat{
		Avg: avg,
		Var: variance,
	}
}

func weightedAvg(l, w []int64) int64 {
	var v, W int64
	for i := range w {
		v += w[i] * l[i]
		W += w[i]
	}
	if W == 0 {
		return 0
	}
	return v / W
}

func weightedVar(avg int64, avgs, vars, w []int64) int64 {
	var v, W int64
	for i := range w {
		v += w[i] * (vars[i] + square(avgs[i]-avg))
		W += w[i]
	}
	if W == 0 {
		return 0
	}
	return v / W
}
