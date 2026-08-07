package api

import (
	"math"
	"time"

	"github.com/popelev/level2/internal/core"
)

const (
	defaultVerifyTimeoutMs = 2000
	maxVerifyTimeoutMs     = 30000
	verifyPollInterval     = 50 * time.Millisecond
	// Relative + absolute epsilon for float64 analog setpoints (REAL/LREAL round-trip).
	verifyFloatRelEps = 1e-6
	verifyFloatAbsEps = 1e-6
)

func normalizeVerifyTimeoutMs(ms int) time.Duration {
	if ms <= 0 {
		ms = defaultVerifyTimeoutMs
	}
	if ms > maxVerifyTimeoutMs {
		ms = maxVerifyTimeoutMs
	}
	return time.Duration(ms) * time.Millisecond
}

// writeVerifyMatch reports whether observed OPC readback matches the value we intended to write.
// Floats use relative+absolute epsilon; other types require exact SamePayload (quality Good).
func writeVerifyMatch(expected, observed core.Sample, dt core.ValueType) bool {
	if observed.Quality != core.QualityGood {
		return false
	}
	dt = core.NormalizeValueType(dt)
	switch dt {
	case core.ValueFloat64:
		if expected.ValueNum == nil || observed.ValueNum == nil {
			return false
		}
		return floatNearlyEqual(*expected.ValueNum, *observed.ValueNum)
	default:
		// Ignore timestamp differences; require Good quality on both sides for exact types.
		exp := expected
		exp.Quality = core.QualityGood
		obs := observed
		return exp.SamePayload(obs)
	}
}

func floatNearlyEqual(a, b float64) bool {
	if a == b {
		return true
	}
	diff := math.Abs(a - b)
	if diff <= verifyFloatAbsEps {
		return true
	}
	scale := math.Max(math.Abs(a), math.Abs(b))
	return diff <= verifyFloatRelEps*math.Max(1, scale)
}
