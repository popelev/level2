package opcua

import (
	"fmt"
	"math"
	"time"

	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

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
		str := tm.UTC().Format(time.RFC3339Nano)
		s.ValueText = &str
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
	default:
		return time.Time{}, fmt.Errorf("expected datetime, got %T", v)
	}
}
