package opcua

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

// OPC UA DateTime / Windows FILETIME: 100-ns ticks since 1601-01-01 UTC.
const opcFiletimeEpoch = int64(116444736000000000)

// ErrStructureNode indicates the OPC node is a custom structure / extension object.
var ErrStructureNode = fmt.Errorf("node is a structure/extension object; use leaf node or expand (M2)")

func mapDataValue(tag TagView, dv *ua.DataValue, now time.Time) (core.Sample, error) {
	s := core.Sample{Time: now, TagID: tag.ID, Quality: core.QualityBad}
	if dv == nil {
		return s, fmt.Errorf("nil DataValue")
	}
	if dv.Status != ua.StatusOK {
		// Siemens structures often surface as BadDataTypeIDUnknown
		if dv.Status == ua.StatusCode(0x80110000) { // BadDataTypeIDUnknown
			return s, fmt.Errorf("%w: status %v", ErrStructureNode, dv.Status)
		}
		return s, fmt.Errorf("opc status %v", dv.Status)
	}
	s.Quality = core.QualityGood
	if dv.SourceTimestamp != (time.Time{}) {
		s.Time = dv.SourceTimestamp.UTC()
	} else if dv.ServerTimestamp != (time.Time{}) {
		s.Time = dv.ServerTimestamp.UTC()
	}

	if dv.Value == nil {
		return s, fmt.Errorf("empty value")
	}
	v := dv.Value.Value()

	switch tag.DataType {
	case core.ValueFloat64:
		f, err := asFloat64(v)
		if err != nil {
			return s, err
		}
		s.ValueNum = &f
	case core.ValueInt64:
		n, err := asInt64(v)
		if err != nil {
			return s, err
		}
		f := float64(n)
		s.ValueNum = &f
	case core.ValueUint:
		n, err := asUint64(v)
		if err != nil {
			return s, err
		}
		f := float64(n)
		s.ValueNum = &f
	case core.ValueBool:
		b, ok := v.(bool)
		if !ok {
			return s, fmt.Errorf("expected bool, got %T", v)
		}
		s.ValueBool = &b
	case core.ValueString:
		str, err := asString(v)
		if err != nil {
			return s, err
		}
		str, _ = core.TruncateString(str)
		s.ValueText = &str
	case core.ValueDateTime:
		tm, err := asDateTime(v)
		if err != nil {
			return s, err
		}
		if tm.IsZero() {
			// Null / unset Siemens DT (all-zero ByteArray) — Good with empty text.
			empty := ""
			s.ValueText = &empty
		} else {
			str := tm.UTC().Format(time.RFC3339Nano)
			s.ValueText = &str
		}
	default:
		return s, fmt.Errorf("unsupported datatype %q", tag.DataType)
	}
	return s, nil
}

func asFloat64(v any) (float64, error) {
	switch x := v.(type) {
	case float32:
		return float64(x), nil
	case float64:
		return x, nil
	case int16:
		return float64(x), nil
	case int32:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case uint16:
		return float64(x), nil
	case uint32:
		return float64(x), nil
	case uint64:
		if x > math.MaxInt64 {
			return 0, fmt.Errorf("uint64 overflow")
		}
		return float64(x), nil
	default:
		return 0, fmt.Errorf("%w or wrong type %T (want float)", ErrStructureNode, v)
	}
}

func asInt64(v any) (int64, error) {
	switch x := v.(type) {
	case int8:
		return int64(x), nil
	case int16:
		return int64(x), nil
	case int32:
		return int64(x), nil
	case int64:
		return x, nil
	case uint8:
		return int64(x), nil
	case uint16:
		return int64(x), nil
	case uint32:
		return int64(x), nil
	case uint64:
		if x > math.MaxInt64 {
			return 0, fmt.Errorf("uint64 overflow")
		}
		return int64(x), nil
	default:
		return 0, fmt.Errorf("expected int, got %T", v)
	}
}

func asUint64(v any) (uint64, error) {
	switch x := v.(type) {
	case uint8:
		return uint64(x), nil
	case uint16:
		return uint64(x), nil
	case uint32:
		return uint64(x), nil
	case uint64:
		return x, nil
	case int8:
		if x < 0 {
			return 0, fmt.Errorf("negative value for uint")
		}
		return uint64(x), nil
	case int16:
		if x < 0 {
			return 0, fmt.Errorf("negative value for uint")
		}
		return uint64(x), nil
	case int32:
		if x < 0 {
			return 0, fmt.Errorf("negative value for uint")
		}
		return uint64(x), nil
	case int64:
		if x < 0 {
			return 0, fmt.Errorf("negative value for uint")
		}
		return uint64(x), nil
	default:
		return 0, fmt.Errorf("expected uint, got %T", v)
	}
}

func asString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case []byte:
		return string(x), nil
	default:
		return "", fmt.Errorf("expected string, got %T", v)
	}
}

func asDateTime(v any) (time.Time, error) {
	switch x := v.(type) {
	case time.Time:
		return x, nil
	case *time.Time:
		if x == nil {
			return time.Time{}, fmt.Errorf("nil time")
		}
		return *x, nil
	case int64:
		return numericToTime(x), nil
	case int32:
		return numericToTime(int64(x)), nil
	case uint64:
		return numericToTime(int64(x)), nil
	case uint32:
		return numericToTime(int64(x)), nil
	case float64:
		// Some stacks surface FILETIME / unix as float64.
		return numericToTime(int64(x)), nil
	case string:
		return parseDateTimeString(x)
	case []byte:
		return bytesToDateTime(x)
	case ua.ByteArray:
		// Siemens S7 DATE_AND_TIME often arrives as ByteArray (BCD), not OPC DateTime.
		return bytesToDateTime([]byte(x))
	default:
		if b, ok := asByteSlice(v); ok {
			return bytesToDateTime(b)
		}
		// ua.DateTime (older gopcua) or any int64-backed / Time()-providing type.
		if tm, ok := timeFromReflect(v); ok {
			return tm, nil
		}
		return time.Time{}, fmt.Errorf("expected datetime, got %T", v)
	}
}

func asByteSlice(v any) ([]byte, bool) {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		if rv.Type().Elem().Kind() != reflect.Uint8 {
			return nil, false
		}
		out := make([]byte, rv.Len())
		reflect.Copy(reflect.ValueOf(out), rv)
		return out, true
	default:
		return nil, false
	}
}

func bytesToDateTime(b []byte) (time.Time, error) {
	if len(b) == 8 && isAllZero(b) {
		// Null OPC DateTime / uninitialized Siemens DT — successful read, empty time.
		return time.Time{}, nil
	}
	if tm, err := siemensDateAndTime(b); err == nil {
		return tm, nil
	}
	if len(b) == 8 {
		// Raw OPC UA DateTime / Windows FILETIME (little-endian int64).
		ft := int64(binary.LittleEndian.Uint64(b))
		if tm := filetimeToTime(ft); !tm.IsZero() && tm.Year() >= 1990 && tm.Year() <= 2100 {
			return tm, nil
		}
		if tm, err := siemensLDT(b); err == nil {
			return tm, nil
		}
	}
	if tm, err := parseDateTimeString(string(b)); err == nil {
		return tm, nil
	}
	return time.Time{}, fmt.Errorf("cannot decode datetime bytes (len=%d hex=%x)", len(b), b)
}

func isAllZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return len(b) > 0
}

// siemensLDT decodes S7-1500 LDT (nanoseconds since Unix epoch, big-endian).
func siemensLDT(b []byte) (time.Time, error) {
	if len(b) != 8 {
		return time.Time{}, fmt.Errorf("LDT needs 8 bytes")
	}
	var nano int64
	for _, x := range b {
		nano = (nano << 8) | int64(x)
	}
	// Also try little-endian (OPC UA arrays sometimes LE).
	le := int64(uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56)
	for _, n := range []int64{nano, le} {
		if n == 0 {
			continue
		}
		tm := time.Unix(0, n).UTC()
		// Plausible industrial range.
		if tm.Year() >= 1990 && tm.Year() <= 2100 {
			return tm, nil
		}
	}
	return time.Time{}, fmt.Errorf("LDT out of range")
}

// siemensDateAndTime decodes S7 DATE_AND_TIME (DT) — 8 BCD bytes.
// Layout matches gos7 Helper.GetDateTimeAt / Siemens DT online format.
func siemensDateAndTime(b []byte) (time.Time, error) {
	if len(b) != 8 {
		return time.Time{}, fmt.Errorf("siemens DT needs 8 bytes, got %d", len(b))
	}
	// Reject clearly non-BCD nibbles early (values > 9).
	for i := 0; i < 6; i++ {
		if (b[i]>>4) > 9 || (b[i]&0x0F) > 9 {
			return time.Time{}, fmt.Errorf("non-BCD byte at %d: %02x", i, b[i])
		}
	}
	year := decodeBCD(b[0])
	if year < 90 {
		year += 2000
	} else {
		year += 1900
	}
	month := decodeBCD(b[1])
	day := decodeBCD(b[2])
	hour := decodeBCD(b[3])
	min := decodeBCD(b[4])
	sec := decodeBCD(b[5])
	msec := decodeBCD(b[6])*10 + int(b[7]>>4)
	if month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || min > 59 || sec > 59 || msec > 999 {
		return time.Time{}, fmt.Errorf("invalid siemens DT fields y=%d m=%d d=%d %02d:%02d:%02d.%03d hex=%x",
			year, month, day, hour, min, sec, msec, b)
	}
	return time.Date(year, time.Month(month), day, hour, min, sec, msec*1_000_000, time.UTC), nil
}

func decodeBCD(b byte) int {
	return int((b>>4)*10 + (b & 0x0F))
}

func filetimeToTime(ft int64) time.Time {
	if ft == 0 {
		return time.Time{}
	}
	// Same conversion as gopcua ua.Buffer.ReadTime.
	return time.Unix(0, (ft-opcFiletimeEpoch)*100).UTC()
}

// numericToTime accepts OPC FILETIME ticks or unix seconds/millis.
func numericToTime(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	if n > opcFiletimeEpoch/10 { // FILETIME-scale (≃ after ~year 0160)
		return filetimeToTime(n)
	}
	if n > 1_000_000_000_000 { // unix millis
		return time.UnixMilli(n).UTC()
	}
	return time.Unix(n, 0).UTC()
}

func parseDateTimeString(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty datetime string")
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if tm, err := time.Parse(layout, s); err == nil {
			return tm.UTC(), nil
		}
	}
	// Numeric FILETIME or unix seconds/millis encoded as string.
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return numericToTime(n), nil
	}
	return time.Time{}, fmt.Errorf("cannot parse datetime %q", s)
}

func timeFromReflect(v any) (time.Time, bool) {
	if v == nil {
		return time.Time{}, false
	}
	rv := reflect.ValueOf(v)
	// Method Time() time.Time (e.g. historical ua.DateTime).
	if m := rv.MethodByName("Time"); m.IsValid() && m.Type().NumIn() == 0 && m.Type().NumOut() == 1 {
		if m.Type().Out(0) == reflect.TypeOf(time.Time{}) {
			out := m.Call(nil)
			return out[0].Interface().(time.Time), true
		}
	}
	// Named type with underlying int64 / uint64 (FILETIME).
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return time.Time{}, false
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int64, reflect.Int32:
		return numericToTime(rv.Int()), true
	case reflect.Uint, reflect.Uint64, reflect.Uint32:
		return numericToTime(int64(rv.Uint())), true
	}
	return time.Time{}, false
}
