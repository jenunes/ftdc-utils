package ftdc

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func writeVarint(buf *bytes.Buffer, v uint64) {
	for v >= 0x80 {
		buf.WriteByte(byte(v&0x7F) | 0x80)
		v >>= 7
	}
	buf.WriteByte(byte(v))
}

func buildMetricChunkDoc(refDoc bson.D, nmetrics, ndeltas int, deltas []uint64) (bson.D, error) {
	refBytes, err := bson.Marshal(refDoc)
	if err != nil {
		return nil, err
	}

	var uncompressed bytes.Buffer
	uncompressed.Write(refBytes)

	nm := make([]byte, 4)
	binary.LittleEndian.PutUint32(nm, uint32(nmetrics))
	uncompressed.Write(nm)

	nd := make([]byte, 4)
	binary.LittleEndian.PutUint32(nd, uint32(ndeltas))
	uncompressed.Write(nd)

	// Write deltas as varints (metrics-major order)
	for _, d := range deltas {
		writeVarint(&uncompressed, d)
	}

	// Compress with zlib
	var compressed bytes.Buffer
	w, err := zlib.NewWriterLevel(&compressed, zlib.BestSpeed)
	if err != nil {
		return nil, err
	}
	_, err = w.Write(uncompressed.Bytes())
	if err != nil {
		return nil, err
	}
	err = w.Close()
	if err != nil {
		return nil, err
	}

	// Build the data field: 4-byte uncompressed length + compressed data
	var data bytes.Buffer
	ul := make([]byte, 4)
	binary.LittleEndian.PutUint32(ul, uint32(uncompressed.Len()))
	data.Write(ul)
	data.Write(compressed.Bytes())

	doc := bson.D{
		{Key: "_id", Value: bson.DateTime(0)},
		{Key: "type", Value: int32(1)},
		{Key: "data", Value: bson.Binary{Subtype: 0x00, Data: data.Bytes()}},
	}
	return doc, nil
}

func TestReadChunks_DeltaOrder(t *testing.T) {
	t.Parallel()

	refDoc := bson.D{
		{Key: "a", Value: int32(10)},
		{Key: "b", Value: int32(20)},
	}
	// 2 metrics, 3 deltas
	// Metrics-major: a's deltas first, then b's deltas
	// a: 1, 2, 3
	// b: 10, 20, 30
	deltas := []uint64{1, 2, 3, 10, 20, 30}

	doc, err := buildMetricChunkDoc(refDoc, 2, 3, deltas)
	if err != nil {
		t.Fatal(err)
	}

	docBytes, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	chunks := make(chan Chunk, 1)
	bsonDocs := make(chan bson.D, 1)

	var parsedDoc bson.D
	err = bson.Unmarshal(docBytes, &parsedDoc)
	if err != nil {
		t.Fatal(err)
	}
	bsonDocs <- parsedDoc
	close(bsonDocs)

	abrt := make(chan bool)
	err = readChunks(bsonDocs, chunks, abrt)
	if err != nil {
		t.Fatal(err)
	}

	var chunk Chunk
	var gotChunk bool
	for c := range chunks {
		chunk = c
		gotChunk = true
	}
	if !gotChunk {
		t.Fatal("no chunk produced")
	}

	if len(chunk.Metrics) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(chunk.Metrics))
	}
	if chunk.NDeltas != 3 {
		t.Fatalf("expected 3 deltas, got %d", chunk.NDeltas)
	}

	aDeltas := chunk.Metrics[0].Deltas
	bDeltas := chunk.Metrics[1].Deltas
	wantA := []int64{1, 2, 3}
	wantB := []int64{10, 20, 30}

	for i, w := range wantA {
		if aDeltas[i] != w {
			t.Errorf("metric a delta[%d]: expected %d, got %d", i, w, aDeltas[i])
		}
	}
	for i, w := range wantB {
		if bDeltas[i] != w {
			t.Errorf("metric b delta[%d]: expected %d, got %d", i, w, bDeltas[i])
		}
	}
}

func TestReadChunks_RLEZeros(t *testing.T) {
	t.Parallel()

	refDoc := bson.D{
		{Key: "x", Value: int32(100)},
	}

	// 1 metric, 5 deltas -- all zeros
	// RLE encoding: first delta is 0, then zero-count = 4 (remaining zeros)
	var deltasBuf bytes.Buffer
	writeVarint(&deltasBuf, 0) // delta = 0 triggers RLE
	writeVarint(&deltasBuf, 4) // 4 more zeros after the first

	doc, err := buildMetricChunkDocRaw(refDoc, 1, 5, deltasBuf.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	docBytes, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	chunks := make(chan Chunk, 1)
	bsonDocs := make(chan bson.D, 1)

	var parsedDoc bson.D
	err = bson.Unmarshal(docBytes, &parsedDoc)
	if err != nil {
		t.Fatal(err)
	}
	bsonDocs <- parsedDoc
	close(bsonDocs)

	abrt := make(chan bool)
	err = readChunks(bsonDocs, chunks, abrt)
	if err != nil {
		t.Fatal(err)
	}

	var chunk Chunk
	for c := range chunks {
		chunk = c
	}

	if len(chunk.Metrics[0].Deltas) != 5 {
		t.Fatalf("expected 5 deltas, got %d", len(chunk.Metrics[0].Deltas))
	}
	for i, d := range chunk.Metrics[0].Deltas {
		if d != 0 {
			t.Errorf("delta[%d]: expected 0, got %d", i, d)
		}
	}
}

func buildMetricChunkDocRaw(refDoc bson.D, nmetrics, ndeltas int, rawDeltas []byte) (bson.D, error) {
	refBytes, err := bson.Marshal(refDoc)
	if err != nil {
		return nil, err
	}

	var uncompressed bytes.Buffer
	uncompressed.Write(refBytes)

	nm := make([]byte, 4)
	binary.LittleEndian.PutUint32(nm, uint32(nmetrics))
	uncompressed.Write(nm)

	nd := make([]byte, 4)
	binary.LittleEndian.PutUint32(nd, uint32(ndeltas))
	uncompressed.Write(nd)

	uncompressed.Write(rawDeltas)

	var compressed bytes.Buffer
	w, err := zlib.NewWriterLevel(&compressed, zlib.BestSpeed)
	if err != nil {
		return nil, err
	}
	_, err = w.Write(uncompressed.Bytes())
	if err != nil {
		return nil, err
	}
	err = w.Close()
	if err != nil {
		return nil, err
	}

	var data bytes.Buffer
	ul := make([]byte, 4)
	binary.LittleEndian.PutUint32(ul, uint32(uncompressed.Len()))
	data.Write(ul)
	data.Write(compressed.Bytes())

	doc := bson.D{
		{Key: "_id", Value: bson.DateTime(0)},
		{Key: "type", Value: int32(1)},
		{Key: "data", Value: bson.Binary{Subtype: 0x00, Data: data.Bytes()}},
	}
	return doc, nil
}

func TestReadChunks_TypeDispatch(t *testing.T) {
	t.Parallel()

	type0 := bson.D{
		{Key: "_id", Value: bson.DateTime(0)},
		{Key: "type", Value: int32(0)},
		{Key: "doc", Value: bson.D{{Key: "buildInfo", Value: bson.D{}}}},
	}
	type2 := bson.D{
		{Key: "_id", Value: bson.DateTime(0)},
		{Key: "type", Value: int32(2)},
		{Key: "count", Value: int32(5)},
		{Key: "doc", Value: bson.D{{Key: "param", Value: int32(1)}}},
	}

	chunks := make(chan Chunk, 10)
	bsonDocs := make(chan bson.D, 10)

	bsonDocs <- type0
	bsonDocs <- type2
	close(bsonDocs)

	abrt := make(chan bool)
	err := readChunks(bsonDocs, chunks, abrt)
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for range chunks {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 chunks from type-0 and type-2 docs, got %d", count)
	}
}

func TestReadChunks_CorruptInput(t *testing.T) {
	t.Parallel()

	doc := bson.D{
		{Key: "_id", Value: bson.DateTime(0)},
		{Key: "type", Value: int32(1)},
		{Key: "data", Value: bson.Binary{Subtype: 0x00, Data: []byte{0, 0, 0}}},
	}

	chunks := make(chan Chunk, 1)
	bsonDocs := make(chan bson.D, 1)
	bsonDocs <- doc
	close(bsonDocs)

	abrt := make(chan bool)
	err := readChunks(bsonDocs, chunks, abrt)
	if err == nil {
		t.Error("expected error for corrupt/truncated data")
	}
}

func TestReadDiagnostic_EmptyInput(t *testing.T) {
	t.Parallel()

	ch := make(chan bson.D, 1)
	abrt := make(chan bool)

	err := readDiagnostic(bytes.NewReader(nil), ch, abrt)
	if err != nil {
		t.Errorf("expected nil error for empty input, got %v", err)
	}

	count := 0
	for range ch {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 docs, got %d", count)
	}
}

func TestReadDiagnostic_ValidBSON(t *testing.T) {
	t.Parallel()

	doc := bson.D{{Key: "type", Value: int32(0)}, {Key: "doc", Value: bson.D{}}}
	docBytes, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	ch := make(chan bson.D, 10)
	abrt := make(chan bool)

	err = readDiagnostic(bytes.NewReader(docBytes), ch, abrt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	count := 0
	for range ch {
		count++
	}
	if count != 1 {
		t.Errorf("expected 1 doc, got %d", count)
	}
}

func TestReadBufDoc_InvalidLength(t *testing.T) {
	t.Parallel()

	// BSON doc with length 0x7FFFFFFF (way too large)
	data := []byte{0xFF, 0xFF, 0xFF, 0x7F}
	buf := bufio.NewReader(bytes.NewReader(data))
	var doc bson.D
	err := readBufDoc(buf, &doc)
	if err == nil {
		t.Error("expected error for oversized BSON length")
	}
}
