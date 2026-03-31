package ftdc

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// CmpThreshold is the threshold for comparison of metrics used by the
// Proximal function.
var CmpThreshold float64 = 0.2

var cmpMetrics = map[string]bool{
	"end":                                            true,
	"start":                                          true,
	"serverStatus.start":                             true,
	"serverStatus.end":                               true,
	"serverStatus.asserts":                           true,
	"serverStatus.connections":                       true,
	"serverStatus.mem.resident":                      true,
	"serverStatus.mem.supported":                     true,
	"serverStatus.mem.virtual":                       true,
	"serverStatus.metrics.commands":                  true,
	"serverStatus.metrics.cursor.open":               true,
	"serverStatus.metrics.document":                  true,
	"serverStatus.metrics.operation":                 true,
	"serverStatus.metrics.queryExecutor":             true,
	"serverStatus.metrics.repl":                      true,
	"serverStatus.metrics.storage":                   true,
	"serverStatus.metrics.ttl":                       true,
	"serverStatus.opcounters":                        true,
	"serverStatus.opcountersRepl":                    true,
	"serverStatus.opLatencies":                       true,
	"serverStatus.tcmalloc":                          true,
	"serverStatus.transactions":                      true,
	"serverStatus.flowControl":                       true,
	"serverStatus.wiredTiger.LSM":                    true,
	"serverStatus.wiredTiger.async":                  true,
	"serverStatus.wiredTiger.block-manager":          true,
	"serverStatus.wiredTiger.cache":                  true,
	"serverStatus.wiredTiger.concurrentTransactions": true,
	"serverStatus.wiredTiger.data-handle":            true,
	"serverStatus.wiredTiger.reconciliation":         true,
	"serverStatus.wiredTiger.session":                true,
	"systemMetrics":                                  true,
}

const badTimePenalty = -0.1

// CmpScore holds information for the comparison of a single metric.
type CmpScore struct {
	// Metric is the name of the metric being compared
	Metric string

	// Score is the value of the score, in the range [0, 1]
	Score float64

	// Error stores an error message if the threshold was not met
	Err error
}

// CmpScores implements sort.Interface for CmpScore slices
type CmpScores []CmpScore

func (s CmpScores) Len() int {
	return len(s)
}
func (s CmpScores) Less(i, j int) bool {
	return s[i].Score < s[j].Score
}
func (s CmpScores) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func isCmpMetric(key string) bool {
	s := strings.Split(key, ".")
	for i := range s {
		prefix := strings.Join(s[:i+1], ".")
		if _, ok := cmpMetrics[prefix]; ok {
			return true
		}
	}
	return false
}

// Proximal computes a measure of deviation between two sets of metric
// statistics. It computes an aggregated score based on compareMetrics
// output, and compares it against the CmpThreshold.
//
// Return values: score holds the numeric rating (1.0 = perfect), scores is
// the sorted list of scores for all compared metrics, and ok is whether the
// threshold was met.
func Proximal(a, b Stats) (score float64, scores CmpScores, ok bool) {
	aCount := float64(a.NSamples)
	bCount := float64(b.NSamples)
	diff := math.Abs(aCount - bCount)
	max := math.Max(aCount, bCount)
	nsampleScore := CmpScore{
		Metric: "NSamples",
		Score:  1,
	}
	if max > 0 && diff/max > CmpThreshold {
		nsampleScore.Score = 1 + 2*badTimePenalty
		nsampleScore.Err = fmt.Errorf("sample count not proximal: (%d, %d) "+
			"are not within threshold (%d%%)\n",
			a.NSamples, b.NSamples, int(CmpThreshold*100))
	}

	scores = make(CmpScores, 0)
	scores = append(scores, nsampleScore)
	for key := range a.Metrics {
		if _, ok := b.Metrics[key]; !ok {
			continue
		}
		if !isCmpMetric(key) {
			continue
		}
		cmp := compareMetrics(a, b, key)
		scores = append(scores, cmp)
	}
	sort.Sort(scores)

	for i, c := range scores {
		score += math.Pow(2, -float64(i+1)) * c.Score
	}
	score = math.Sqrt(score)

	ok = score >= (1 - CmpThreshold)
	return
}

// compareMetrics computes a measure of deviation between two samples of the
// same metric.
func compareMetrics(sa, sb Stats, key string) (score CmpScore) {
	score.Metric = key
	a := sa.Metrics[key]
	b := sb.Metrics[key]
	if a.Avg == b.Avg {
		score.Score = 1
		return
	}
	maxavg := math.Max(math.Abs(float64(a.Avg)), math.Abs(float64(b.Avg)))
	maxvar := math.Max(math.Abs(float64(a.Var)), math.Abs(float64(b.Var)))
	if maxavg == 0 || maxvar == 0 {
		score.Score = 1
		return
	}

	relavg := math.Abs(float64(a.Avg-b.Avg)) / maxavg
	relvar := math.Abs(float64(a.Var-b.Var)) / maxvar
	score.Score = math.Abs((1 - relavg) * (1 - relvar))

	var msg string
	if relavg > CmpThreshold {
		msg = fmt.Sprintf("metric '%s' not proximal: "+
			"averages (%d, %d) are not within threshold (%d%%)\n",
			key, a.Avg, b.Avg, int(CmpThreshold*100))
	}
	if relvar > CmpThreshold {
		msg += fmt.Sprintf("metric '%s' not proximal: "+
			"variances (%d, %d) are not within threshold (%d%%)\n",
			key, a.Var, b.Var, int(CmpThreshold*100))
	}
	if msg != "" {
		score.Err = fmt.Errorf("%s", msg)
	}
	return
}
