package core

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// CoerceWriteValue converts a JSON-decoded value into a Go scalar suitable for protocol write
// (e.g. OPC UA ua.NewVariant), using the tag's configured datatype.
func CoerceWriteValue(dt ValueType, v any) (any, error) {
	dt = NormalizeValueType(dt)
	if dt == "" {
		return nil, fmt.Errorf("datatype required")
	}
	if v == nil {
		return nil, fmt.Errorf("value is required")
	}
	switch dt {
	case ValueBool:
		return coerceBool(v)
	case ValueInt64:
		return coerceInt64(v)
	case ValueUint:
		return coerceUint32(v)
	case ValueFloat64:
		return coerceFloat64(v)
	case ValueString:
		return coerceString(v)
	case ValueDateTime:
		return coerceDateTime(v)
	default:
		return nil, fmt.Errorf("unsupported datatype %q", dt)
	}
}

func coerceBool(v any) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case float64:
		if x == 1 {
			return true, nil
		}
		if x == 0 {
			return false, nil
		}
		return false, fmt.Errorf("bool from number must be 0 or 1")
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "1", "yes", "on":
			return true, nil
		case "false", "0", "no", "off":
			return false, nil
		default:
			return false, fmt.Errorf("cannot parse bool %q", x)
		}
	default:
		return false, fmt.Errorf("expected bool, got %T", v)
	}
}

func coerceInt64(v any) (int64, error) {
	switch x := v.(type) {
	case float64:
		if math.Trunc(x) != x {
			return 0, fmt.Errorf("int64 requires whole number")
		}
		if x > float64(math.MaxInt64) || x < float64(math.MinInt64) {
			return 0, fmt.Errorf("int64 overflow")
		}
		return int64(x), nil
	case int64:
		return x, nil
	case int:
		return int64(x), nil
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot parse int64 %q", x)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("expected int64, got %T", v)
	}
}

func coerceUint32(v any) (uint32, error) {
	switch x := v.(type) {
	case float64:
		if x < 0 || math.Trunc(x) != x {
			return 0, fmt.Errorf("uint requires non-negative whole number")
		}
		if x > float64(math.MaxUint32) {
			return 0, fmt.Errorf("uint32 overflow")
		}
		return uint32(x), nil
	case int64:
		if x < 0 {
			return 0, fmt.Errorf("uint requires non-negative")
		}
		if x > math.MaxUint32 {
			return 0, fmt.Errorf("uint32 overflow")
		}
		return uint32(x), nil
	case int:
		if x < 0 {
			return 0, fmt.Errorf("uint requires non-negative")
		}
		return uint32(x), nil
	case string:
		n, err := strconv.ParseUint(strings.TrimSpace(x), 10, 32)
		if err != nil {
			return 0, fmt.Errorf("cannot parse uint %q", x)
		}
		return uint32(n), nil
	default:
		return 0, fmt.Errorf("expected uint, got %T", v)
	}
}

func coerceFloat64(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case int64:
		return float64(x), nil
	case int:
		return float64(x), nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return 0, fmt.Errorf("cannot parse float64 %q", x)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("expected float64, got %T", v)
	}
}

func coerceString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		s, _ := TruncateString(x)
		return s, nil
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64), nil
	case bool:
		if x {
			return "true", nil
		}
		return "false", nil
	default:
		return "", fmt.Errorf("expected string, got %T", v)
	}
}

func coerceDateTime(v any) (time.Time, error) {
	switch x := v.(type) {
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return time.Time{}, fmt.Errorf("datetime value required")
		}
		if tm, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return tm.UTC(), nil
		}
		if tm, err := time.Parse(time.RFC3339, s); err == nil {
			return tm.UTC(), nil
		}
		return time.Time{}, fmt.Errorf("datetime must be RFC3339, got %q", x)
	case time.Time:
		return x.UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("expected datetime string, got %T", v)
	}
}
