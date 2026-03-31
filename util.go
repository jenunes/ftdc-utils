package ftdc

import (
	"bufio"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func flattenBSON(d bson.D) []Metric {
	return flattenBSONPrefix(d, "")
}

func flattenBSONPrefix(d bson.D, prefix string) []Metric {
	var o []Metric
	for _, e := range d {
		key := e.Key
		if prefix != "" {
			key = prefix + "." + e.Key
		}
		switch child := e.Value.(type) {
		case bson.D:
			o = append(o, flattenBSONPrefix(child, key)...)
		case bson.A:
			sub := make(bson.D, len(child))
			for i, v := range child {
				sub[i] = bson.E{Key: string(rune('0' + i)), Value: v}
			}
			o = append(o, flattenBSONPrefix(sub, key)...)
		case string:
			// skip non-numeric
		case bool:
			var v int64
			if child {
				v = 1
			}
			o = append(o, Metric{Key: key, Value: v})
		case float64:
			var v int64
			if math.IsNaN(child) {
				v = 0
			} else if child >= math.MaxInt64 {
				v = math.MaxInt64
			} else if child <= math.MinInt64 {
				v = math.MinInt64
			} else {
				v = int64(child)
			}
			o = append(o, Metric{Key: key, Value: v})
		case int32:
			o = append(o, Metric{Key: key, Value: int64(child)})
		case int64:
			o = append(o, Metric{Key: key, Value: child})
		case time.Time:
			o = append(o, Metric{Key: key, Value: child.UnixMilli()})
		case bson.DateTime:
			o = append(o, Metric{Key: key, Value: int64(child)})
		case bson.Timestamp:
			o = append(o, Metric{Key: key, Value: int64(child.T)})
			o = append(o, Metric{Key: key, Value: int64(child.I)})
		case bson.Decimal128:
			bi := decimal128ToInt64(child)
			o = append(o, Metric{Key: key, Value: bi})
		}
	}
	return o
}

func decimal128ToInt64(d bson.Decimal128) int64 {
	bi, _, err := d.BigInt()
	if err != nil || bi == nil {
		return 0
	}
	if bi.IsInt64() {
		return bi.Int64()
	}
	if bi.Sign() > 0 {
		return math.MaxInt64
	}
	return math.MinInt64
}

// unpackDelta reads a varint-encoded uint64 from the buffered reader.
// MongoDB FTDC uses unsigned LEB128 (Google Varint) encoding.
func unpackDelta(buf *bufio.Reader) (uint64, error) {
	var res uint64
	var shift uint
	for {
		b, err := buf.ReadByte()
		if err != nil {
			return 0, err
		}
		res |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			return res, nil
		}
		shift += 7
	}
}

func unpackInt(bl []byte) int {
	return int(int32((uint32(bl[0]) << 0) |
		(uint32(bl[1]) << 8) |
		(uint32(bl[2]) << 16) |
		(uint32(bl[3]) << 24)))
}

func sum(l ...int64) int64 {
	var s int64
	for _, v := range l {
		s += v
	}
	return s
}

func square(n int64) int64 {
	return n * n
}

