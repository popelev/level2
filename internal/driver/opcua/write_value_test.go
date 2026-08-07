package opcua

import (
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
)

func TestCoerceWriteValue_OK(t *testing.T) {
	cases := []struct {
		dt   core.ValueType
		in   any
		want any
	}{
		{core.ValueBool, true, true},
		{core.ValueBool, "false", false},
		{core.ValueBool, float64(1), true},
		{core.ValueInt64, float64(42), int64(42)},
		{core.ValueInt64, "7", int64(7)},
		{core.ValueUint, float64(3), uint32(3)},
		{core.ValueUint, "9", uint32(9)},
		{core.ValueFloat64, float64(1.5), float64(1.5)},
		{core.ValueFloat64, "2.25", float64(2.25)},
		{core.ValueString, "hi", "hi"},
		{core.ValueDateTime, "2026-08-06T12:00:00Z", time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		got, err := CoerceWriteValue(tc.dt, tc.in)
		if err != nil {
			t.Fatalf("%s %#v: %v", tc.dt, tc.in, err)
		}
		switch want := tc.want.(type) {
		case time.Time:
			gt, ok := got.(time.Time)
			if !ok || !gt.Equal(want) {
				t.Fatalf("%s: got %v want %v", tc.dt, got, want)
			}
		default:
			if got != want {
				t.Fatalf("%s: got %#v want %#v", tc.dt, got, want)
			}
		}
	}
}

func TestCoerceWriteValue_Reject(t *testing.T) {
	cases := []struct {
		dt core.ValueType
		in any
	}{
		{core.ValueBool, "maybe"},
		{core.ValueBool, float64(2)},
		{core.ValueInt64, float64(1.5)},
		{core.ValueUint, float64(-1)},
		{core.ValueFloat64, true},
		{core.ValueDateTime, ""},
		{core.ValueDateTime, "not-a-date"},
		{core.ValueFloat64, nil},
		{"", 1},
	}
	for _, tc := range cases {
		if _, err := CoerceWriteValue(tc.dt, tc.in); err == nil {
			t.Fatalf("expected error for %s %#v", tc.dt, tc.in)
		}
	}
}
