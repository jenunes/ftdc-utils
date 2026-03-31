package ftdc

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const maxDecompressedSize = 10_000_000

func readDiagnostic(f io.Reader, ch chan<- bson.D, abrt <-chan bool) error {
	defer close(ch)
	buf := bufio.NewReader(f)
	for {
		doc, err := readBufBSON(buf)
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return err
		}
		select {
		case ch <- doc:
		case <-abrt:
			return nil
		}
	}
}

func docMap(doc bson.D) map[string]interface{} {
	m := make(map[string]interface{}, len(doc))
	for _, e := range doc {
		m[e.Key] = e.Value
	}
	return m
}

func readChunks(ch <-chan bson.D, o chan<- Chunk, abrt <-chan bool) error {
	defer close(o)
	for doc := range ch {
		m := docMap(doc)

		rawType, ok := m["type"]
		if !ok {
			continue
		}
		docType := toInt32(rawType)

		switch DocType(docType) {
		case DocMetadata:
			// Type 0: metadata document -- skip for backward compat
			continue
		case DocPeriodicMetadata:
			// Type 2: periodic metadata document -- skip for backward compat
			continue
		case DocMetricChunk:
			// Type 1: compressed metric chunk -- process below
		default:
			fmt.Fprintf(os.Stderr, "Warning: unknown FTDC document type %d, skipping\n", docType)
			continue
		}

		chunk, err := decodeMetricChunk(m)
		if err != nil {
			return err
		}
		select {
		case o <- chunk:
		case <-abrt:
			return nil
		}
	}
	return nil
}

func toInt32(v interface{}) int32 {
	switch n := v.(type) {
	case int32:
		return n
	case int64:
		return int32(n)
	case int:
		return int32(n)
	case float64:
		return int32(n)
	default:
		return -1
	}
}

func decodeMetricChunk(m map[string]interface{}) (Chunk, error) {
	dataRaw, ok := m["data"]
	if !ok {
		return Chunk{}, fmt.Errorf("metric chunk missing 'data' field")
	}

	var zBytes []byte
	switch d := dataRaw.(type) {
	case bson.Binary:
		zBytes = d.Data
	case []byte:
		zBytes = d
	default:
		return Chunk{}, fmt.Errorf("metric chunk 'data' field has unexpected type %T", dataRaw)
	}

	if len(zBytes) < 4 {
		return Chunk{}, fmt.Errorf("metric chunk data too short: %d bytes", len(zBytes))
	}

	// First 4 bytes are the uncompressed length (uint32 LE), followed by zlib data
	uncompressedLen := uint32(zBytes[0]) | uint32(zBytes[1])<<8 | uint32(zBytes[2])<<16 | uint32(zBytes[3])<<24
	if uncompressedLen > maxDecompressedSize {
		return Chunk{}, fmt.Errorf("decompressed size %d exceeds limit %d", uncompressedLen, maxDecompressedSize)
	}

	z, err := zlib.NewReader(bytes.NewReader(zBytes[4:]))
	if err != nil {
		return Chunk{}, fmt.Errorf("zlib open: %w", err)
	}
	defer z.Close()

	buf := bufio.NewReader(z)

	metrics, err := readBufMetrics(buf)
	if err != nil {
		return Chunk{}, fmt.Errorf("reading reference doc: %w", err)
	}

	bl := make([]byte, 8)
	_, err = io.ReadAtLeast(buf, bl, 8)
	if err != nil {
		return Chunk{}, fmt.Errorf("reading metric/delta counts: %w", err)
	}
	nmetrics := unpackInt(bl[:4])
	ndeltas := unpackInt(bl[4:])

	if nmetrics < 0 || ndeltas < 0 {
		return Chunk{}, fmt.Errorf("invalid metric count %d or delta count %d", nmetrics, ndeltas)
	}
	if int64(nmetrics)*int64(ndeltas) > 1_000_000 {
		return Chunk{}, fmt.Errorf("metrics*deltas (%d*%d) exceeds safety limit", nmetrics, ndeltas)
	}

	if nmetrics != len(metrics) {
		fmt.Fprintf(os.Stderr, "Warning: metrics mismatch. Expected %d, got %d\n", nmetrics, len(metrics))
	}

	// Allocate delta storage
	for i := range metrics {
		metrics[i].Deltas = make([]int64, ndeltas)
	}

	// Read deltas in metrics-major order (outer: metrics, inner: samples),
	// matching the MongoDB compressor layout.
	var nzeroes uint64
	for i := 0; i < nmetrics; i++ {
		if i >= len(metrics) {
			break
		}
		for j := 0; j < ndeltas; j++ {
			if nzeroes > 0 {
				metrics[i].Deltas[j] = 0
				nzeroes--
			} else {
				delta, err := unpackDelta(buf)
				if err != nil {
					return Chunk{}, fmt.Errorf("reading delta [metric=%d, sample=%d]: %w", i, j, err)
				}
				if delta == 0 {
					count, err := unpackDelta(buf)
					if err != nil {
						return Chunk{}, fmt.Errorf("reading zero-run count [metric=%d, sample=%d]: %w", i, j, err)
					}
					nzeroes = count
				}
				metrics[i].Deltas[j] = int64(delta)
			}
		}
	}

	return Chunk{
		Metrics: metrics,
		NDeltas: ndeltas,
	}, nil
}

func readBufDoc(buf *bufio.Reader, d interface{}) (err error) {
	var bl []byte
	bl, err = buf.Peek(4)
	if err != nil {
		return
	}
	l := unpackInt(bl)
	if l < 5 || l > maxDecompressedSize {
		return fmt.Errorf("invalid BSON document length: %d", l)
	}

	b := make([]byte, l)
	_, err = io.ReadAtLeast(buf, b, l)
	if err != nil {
		return
	}
	err = bson.Unmarshal(b, d)
	return
}

func readBufBSON(buf *bufio.Reader) (doc bson.D, err error) {
	err = readBufDoc(buf, &doc)
	return
}

func readBufMetrics(buf *bufio.Reader) (metrics []Metric, err error) {
	doc := bson.D{}
	err = readBufDoc(buf, &doc)
	if err != nil {
		return
	}
	metrics = flattenBSON(doc)
	return
}
