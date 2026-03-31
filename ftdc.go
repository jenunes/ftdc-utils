package ftdc

import (
	"io"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// DocType identifies the type of document in an FTDC file.
type DocType int32

const (
	// DocMetadata is a metadata document (type 0), collected on startup/rotation.
	DocMetadata DocType = 0

	// DocMetricChunk is a compressed metric chunk (type 1), the primary data type.
	DocMetricChunk DocType = 1

	// DocPeriodicMetadata is a periodic metadata document (type 2), new in MongoDB ~7.x.
	DocPeriodicMetadata DocType = 2
)

// Chunk represents a 'metric chunk' of data in the FTDC (type 1).
type Chunk struct {
	Metrics []Metric
	NDeltas int
}

// Map converts the chunk to a map representation.
func (c *Chunk) Map() map[string]Metric {
	m := make(map[string]Metric, len(c.Metrics))
	for _, metric := range c.Metrics {
		m[metric.Key] = metric
	}
	return m
}

// Clip trims the chunk to contain as little data as possible while keeping
// data within the given interval. If the chunk is entirely outside of the
// range, it is not modified and the return value is false.
func (c *Chunk) Clip(start, end time.Time) bool {
	st := start.Unix()
	et := end.Unix()
	var si, ei int
	for idx := range c.Metrics {
		m := &c.Metrics[idx]
		if m.Key != "start" {
			continue
		}
		mst := m.Value / 1000
		met := (m.Value + sum(m.Deltas...)) / 1000
		if met < st || mst > et {
			return false
		}
		if mst > st && met < et {
			return true
		}
		t := mst
		for i := 0; i < c.NDeltas; i++ {
			t += m.Deltas[i] / 1000
			if t < st {
				si++
			}
			if t < et {
				ei++
			} else {
				break
			}
		}
		if ei+1 < c.NDeltas {
			ei++
		} else {
			ei = c.NDeltas - 1
		}
		break
	}
	c.NDeltas = ei - si
	for idx := range c.Metrics {
		c.Metrics[idx].Value += sum(c.Metrics[idx].Deltas[:si]...)
		c.Metrics[idx].Deltas = c.Metrics[idx].Deltas[si : ei+1]
	}
	return true
}

// Expand accumulates all deltas to give values of diagnostic data for each
// sample represented by the Chunk. includeKeys specifies which items should be
// included in the output. If a value of includeKeys is false, it won't be
// shown even if the value for a parent document is set to true. If includeKeys
// is nil, data for every key is returned.
func (c *Chunk) Expand(includeKeys map[string]bool) []map[string]int64 {
	deltas := make([]map[string]int64, 0, c.NDeltas+1)
	last := make(map[string]int64, len(c.Metrics))

	for i := -1; i < c.NDeltas; i++ {
		d := make(map[string]int64, len(c.Metrics))
		for _, m := range c.Metrics {
			v, ok := last[m.Key]
			if !ok {
				v = m.Value
			}
			if i > -1 && len(m.Deltas) > 0 {
				v += m.Deltas[i]
			}

			include := true
			if includeKeys != nil {
				var ok bool
				include, ok = includeKeys[m.Key]
				if !ok {
					include = false
					for prefix, inc := range includeKeys {
						if inc && strings.HasPrefix(m.Key, prefix+".") {
							include = true
							break
						}
					}
				}
			}

			if include {
				d[m.Key] = v
			}

			last[m.Key] = v
		}
		deltas = append(deltas, d)
	}

	return deltas
}

// Chunks takes an FTDC diagnostic file in the form of an io.Reader, and
// yields metric chunks (type 1) on the given channel. Type 0 and type 2
// documents are silently skipped for backward compatibility.
// The channel is closed when there are no more chunks.
func Chunks(r io.Reader, c chan<- Chunk) error {
	errCh := make(chan error)
	ch := make(chan bson.D)
	abrt := make(chan bool)
	go func() {
		errCh <- readDiagnostic(r, ch, abrt)
	}()
	go func() {
		errCh <- readChunks(ch, c, abrt)
	}()
	err := <-errCh
	if err != nil {
		close(abrt)
		<-errCh
	} else {
		err = <-errCh
	}
	return err
}

// Metric represents an item in a chunk.
type Metric struct {
	// Key is the dot-delimited key of the metric.
	Key string

	// Value is the value of the metric at the beginning of the sample.
	Value int64

	// Deltas is the slice of deltas, which accumulate on Value to yield the
	// specific sample's value.
	Deltas []int64
}
