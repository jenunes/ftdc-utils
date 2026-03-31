package ftdc

import (
	"bufio"
	"bytes"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestFlattenBSON_Int32(t *testing.T) {
	t.Parallel()
	doc := bson.D{{Key: "a", Value: int32(42)}}
	metrics := flattenBSON(doc)
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	if metrics[0].Key != "a" || metrics[0].Value != 42 {
		t.Errorf("expected a=42, got %s=%d", metrics[0].Key, metrics[0].Value)
	}
}

func TestFlattenBSON_Int64(t *testing.T) {
	t.Parallel()
	doc := bson.D{{Key: "b", Value: int64(1234567890123)}}
	metrics := flattenBSON(doc)
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	if metrics[0].Value != 1234567890123 {
		t.Errorf("expected 1234567890123, got %d", metrics[0].Value)
	}
}

func TestFlattenBSON_Float64(t *testing.T) {
	t.Parallel()
	doc := bson.D{{Key: "f", Value: float64(3.7)}}
	metrics := flattenBSON(doc)
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	if metrics[0].Value != 3 {
		t.Errorf("expected 3 (truncated), got %d", metrics[0].Value)
	}
}

func TestFlattenBSON_Bool(t *testing.T) {
	t.Parallel()
	doc := bson.D{
		{Key: "yes", Value: true},
		{Key: "no", Value: false},
	}
	metrics := flattenBSON(doc)
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(metrics))
	}
	if metrics[0].Value != 1 {
		t.Errorf("true should be 1, got %d", metrics[0].Value)
	}
	if metrics[1].Value != 0 {
		t.Errorf("false should be 0, got %d", metrics[1].Value)
	}
}

func TestFlattenBSON_DateTime(t *testing.T) {
	t.Parallel()
	ts := time.Date(2024, 6, 15, 12, 30, 45, 500_000_000, time.UTC)
	doc := bson.D{{Key: "t", Value: ts}}
	metrics := flattenBSON(doc)
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	expected := ts.UnixMilli()
	if metrics[0].Value != expected {
		t.Errorf("expected %d, got %d", expected, metrics[0].Value)
	}
}

func TestFlattenBSON_BsonDateTime(t *testing.T) {
	t.Parallel()
	dt := bson.DateTime(1718451045500)
	doc := bson.D{{Key: "dt", Value: dt}}
	metrics := flattenBSON(doc)
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	if metrics[0].Value != 1718451045500 {
		t.Errorf("expected 1718451045500, got %d", metrics[0].Value)
	}
}

func TestFlattenBSON_Timestamp(t *testing.T) {
	t.Parallel()
	ts := bson.Timestamp{T: 1718451000, I: 5}
	doc := bson.D{{Key: "ts", Value: ts}}
	metrics := flattenBSON(doc)
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics (seconds + increment), got %d", len(metrics))
	}
	if metrics[0].Value != 1718451000 {
		t.Errorf("expected seconds=1718451000, got %d", metrics[0].Value)
	}
	if metrics[1].Value != 5 {
		t.Errorf("expected increment=5, got %d", metrics[1].Value)
	}
}

func TestFlattenBSON_NestedObject(t *testing.T) {
	t.Parallel()
	doc := bson.D{
		{Key: "outer", Value: bson.D{
			{Key: "inner", Value: int32(99)},
		}},
	}
	metrics := flattenBSON(doc)
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	if metrics[0].Key != "outer.inner" {
		t.Errorf("expected key 'outer.inner', got '%s'", metrics[0].Key)
	}
	if metrics[0].Value != 99 {
		t.Errorf("expected 99, got %d", metrics[0].Value)
	}
}

func TestFlattenBSON_Array(t *testing.T) {
	t.Parallel()
	doc := bson.D{
		{Key: "arr", Value: bson.A{int32(10), int32(20)}},
	}
	metrics := flattenBSON(doc)
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(metrics))
	}
	if metrics[0].Value != 10 || metrics[1].Value != 20 {
		t.Errorf("expected [10, 20], got [%d, %d]", metrics[0].Value, metrics[1].Value)
	}
}

func TestFlattenBSON_StringSkipped(t *testing.T) {
	t.Parallel()
	doc := bson.D{
		{Key: "name", Value: "hello"},
		{Key: "count", Value: int32(5)},
	}
	metrics := flattenBSON(doc)
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric (string skipped), got %d", len(metrics))
	}
	if metrics[0].Key != "count" {
		t.Errorf("expected key 'count', got '%s'", metrics[0].Key)
	}
}

func TestFlattenBSON_Decimal128(t *testing.T) {
	t.Parallel()
	d128, err := bson.ParseDecimal128("12345")
	if err != nil {
		t.Fatalf("failed to create Decimal128: %v", err)
	}
	doc := bson.D{{Key: "dec", Value: d128}}
	metrics := flattenBSON(doc)
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	if metrics[0].Value != 12345 {
		t.Errorf("expected 12345, got %d", metrics[0].Value)
	}
}

func TestFlattenBSON_Mixed(t *testing.T) {
	t.Parallel()
	doc := bson.D{
		{Key: "start", Value: bson.DateTime(1000)},
		{Key: "serverStatus", Value: bson.D{
			{Key: "host", Value: "myhost"},
			{Key: "uptime", Value: int32(3600)},
			{Key: "connections", Value: bson.D{
				{Key: "current", Value: int32(10)},
				{Key: "available", Value: int32(100)},
			}},
		}},
		{Key: "end", Value: bson.DateTime(2000)},
	}
	metrics := flattenBSON(doc)

	keys := make(map[string]int64)
	for _, m := range metrics {
		keys[m.Key] = m.Value
	}

	if keys["start"] != 1000 {
		t.Errorf("start: expected 1000, got %d", keys["start"])
	}
	if keys["end"] != 2000 {
		t.Errorf("end: expected 2000, got %d", keys["end"])
	}
	if keys["serverStatus.uptime"] != 3600 {
		t.Errorf("uptime: expected 3600, got %d", keys["serverStatus.uptime"])
	}
	if keys["serverStatus.connections.current"] != 10 {
		t.Errorf("connections.current: expected 10, got %d", keys["serverStatus.connections.current"])
	}
	if _, ok := keys["serverStatus.host"]; ok {
		t.Error("string field 'host' should have been skipped")
	}
}

func TestUnpackDelta_Zero(t *testing.T) {
	t.Parallel()
	buf := bufio.NewReader(bytes.NewReader([]byte{0x00}))
	v, err := unpackDelta(buf)
	if err != nil {
		t.Fatal(err)
	}
	if v != 0 {
		t.Errorf("expected 0, got %d", v)
	}
}

func TestUnpackDelta_OneByte(t *testing.T) {
	t.Parallel()
	buf := bufio.NewReader(bytes.NewReader([]byte{0x01}))
	v, err := unpackDelta(buf)
	if err != nil {
		t.Fatal(err)
	}
	if v != 1 {
		t.Errorf("expected 1, got %d", v)
	}
}

func TestUnpackDelta_MaxOneByte(t *testing.T) {
	t.Parallel()
	buf := bufio.NewReader(bytes.NewReader([]byte{0x7F}))
	v, err := unpackDelta(buf)
	if err != nil {
		t.Fatal(err)
	}
	if v != 127 {
		t.Errorf("expected 127, got %d", v)
	}
}

func TestUnpackDelta_TwoBytes(t *testing.T) {
	t.Parallel()
	// 300 = 0b100101100 -> LEB128: 0xAC, 0x02
	buf := bufio.NewReader(bytes.NewReader([]byte{0xAC, 0x02}))
	v, err := unpackDelta(buf)
	if err != nil {
		t.Fatal(err)
	}
	if v != 300 {
		t.Errorf("expected 300, got %d", v)
	}
}

func TestUnpackDelta_LargeValue(t *testing.T) {
	t.Parallel()
	// Encode 1_000_000 as unsigned LEB128
	var encoded []byte
	n := uint64(1_000_000)
	for n >= 0x80 {
		encoded = append(encoded, byte(n&0x7F)|0x80)
		n >>= 7
	}
	encoded = append(encoded, byte(n))

	buf := bufio.NewReader(bytes.NewReader(encoded))
	v, err := unpackDelta(buf)
	if err != nil {
		t.Fatal(err)
	}
	if v != 1_000_000 {
		t.Errorf("expected 1000000, got %d", v)
	}
}

func TestUnpackDelta_EOF(t *testing.T) {
	t.Parallel()
	buf := bufio.NewReader(bytes.NewReader([]byte{}))
	_, err := unpackDelta(buf)
	if err == nil {
		t.Error("expected error on empty input")
	}
}

func TestSum(t *testing.T) {
	t.Parallel()
	if s := sum(1, 2, 3, 4, 5); s != 15 {
		t.Errorf("expected 15, got %d", s)
	}
	if s := sum(); s != 0 {
		t.Errorf("expected 0, got %d", s)
	}
	if s := sum(-10, 10); s != 0 {
		t.Errorf("expected 0, got %d", s)
	}
}

func TestSquare(t *testing.T) {
	t.Parallel()
	if s := square(5); s != 25 {
		t.Errorf("expected 25, got %d", s)
	}
	if s := square(-3); s != 9 {
		t.Errorf("expected 9, got %d", s)
	}
	if s := square(0); s != 0 {
		t.Errorf("expected 0, got %d", s)
	}
}

func TestUnpackInt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data []byte
		want int
	}{
		{"zero", []byte{0, 0, 0, 0}, 0},
		{"one", []byte{1, 0, 0, 0}, 1},
		{"256", []byte{0, 1, 0, 0}, 256},
		{"large", []byte{0xD2, 0x02, 0x96, 0x49}, 1234567890},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unpackInt(tt.data)
			if got != tt.want {
				t.Errorf("unpackInt(%v) = %d, want %d", tt.data, got, tt.want)
			}
		})
	}
}

func TestDecimal128ToInt64(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		str  string
		want int64
	}{
		{"zero", "0", 0},
		{"positive", "42", 42},
		{"negative", "-100", -100},
		{"large", "9999999999", 9999999999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d128, err := bson.ParseDecimal128(tt.str)
			if err != nil {
				t.Fatal(err)
			}
			got := decimal128ToInt64(d128)
			if got != tt.want {
				t.Errorf("decimal128ToInt64(%s) = %d, want %d", tt.str, got, tt.want)
			}
		})
	}
}
