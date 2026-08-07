package core

import (
	"math"
	"testing"
	"time"
)

func TestCoerceWriteValue_OK(t *testing.T) {
	cases := []struct {
		dt   ValueType
		in   any
		want any
	}{
		{ValueBool, true, true},
		{ValueBool, "false", false},
		{ValueBool, float64(1), true},
		{ValueInt64, float64(42), int64(42)},
		{ValueInt64, "7", int64(7)},
		{ValueUint, float64(3), uint32(3)},
		{ValueUint, "9", uint32(9)},
		{ValueFloat64, float64(1.5), float64(1.5)},
		{ValueFloat64, "2.25", float64(2.25)},
		{ValueString, "hi", "hi"},
		{ValueDateTime, "2026-08-06T12:00:00Z", time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)},
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
		dt ValueType
		in any
	}{
		{ValueBool, "maybe"},
		{ValueBool, float64(2)},
		{ValueInt64, float64(1.5)},
		{ValueUint, float64(-1)},
		{ValueFloat64, true},
		{ValueDateTime, ""},
		{ValueDateTime, "not-a-date"},
		{ValueFloat64, nil},
		{"", 1},
	}
	for _, tc := range cases {
		if _, err := CoerceWriteValue(tc.dt, tc.in); err == nil {
			t.Fatalf("expected error for %s %#v", tc.dt, tc.in)
		}
	}
}

func TestCoerceWriteValue_Unsupported(t *testing.T) {
	if _, err := CoerceWriteValue("blob", 1); err == nil {
		t.Fatal("unsupported")
	}
}

func TestCoerceBool_Edges(t *testing.T) {
	cases := []struct {
		in   any
		want bool
	}{
		{false, false},
		{"TRUE", true},
		{" Yes ", true},
		{"on", true},
		{"1", true},
		{"NO", false},
		{"off", false},
		{"0", false},
	}
	for _, tc := range cases {
		got, err := coerceBool(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("%#v → %v %v want %v", tc.in, got, err, tc.want)
		}
	}
	if _, err := coerceBool(int(1)); err == nil {
		t.Fatal("int bool")
	}
}

func TestCoerceInt64_Edges(t *testing.T) {
	if v, err := coerceInt64(int64(-9)); err != nil || v != -9 {
		t.Fatalf("int64 %v %v", v, err)
	}
	if v, err := coerceInt64(int(11)); err != nil || v != 11 {
		t.Fatalf("int %v %v", v, err)
	}
	if _, err := coerceInt64(float64(math.MaxFloat64)); err == nil {
		t.Fatal("overflow")
	}
	if _, err := coerceInt64("nope"); err == nil {
		t.Fatal("bad string")
	}
	if _, err := coerceInt64(true); err == nil {
		t.Fatal("bool")
	}
}

func TestCoerceUint32_Edges(t *testing.T) {
	if v, err := coerceUint32(int64(5)); err != nil || v != 5 {
		t.Fatalf("i64 %v %v", v, err)
	}
	if v, err := coerceUint32(int(6)); err != nil || v != 6 {
		t.Fatalf("int %v %v", v, err)
	}
	if _, err := coerceUint32(int64(-1)); err == nil {
		t.Fatal("neg i64")
	}
	if _, err := coerceUint32(int(-1)); err == nil {
		t.Fatal("neg int")
	}
	if _, err := coerceUint32(int64(math.MaxUint32) + 1); err == nil {
		t.Fatal("i64 overflow")
	}
	if _, err := coerceUint32(float64(math.MaxUint32) + 1); err == nil {
		t.Fatal("f64 overflow")
	}
	if _, err := coerceUint32(float64(1.5)); err == nil {
		t.Fatal("frac")
	}
	if _, err := coerceUint32("nope"); err == nil {
		t.Fatal("bad string")
	}
	if _, err := coerceUint32(true); err == nil {
		t.Fatal("bool")
	}
	if v, err := coerceUint32(float64(math.MaxUint32)); err != nil || v != math.MaxUint32 {
		t.Fatalf("max %v %v", v, err)
	}
}

func TestCoerceFloat64_Edges(t *testing.T) {
	if v, err := coerceFloat64(int64(3)); err != nil || v != 3 {
		t.Fatalf("i64 %v %v", v, err)
	}
	if v, err := coerceFloat64(int(4)); err != nil || v != 4 {
		t.Fatalf("int %v %v", v, err)
	}
	if v, err := coerceFloat64(" 3.5 "); err != nil || v != 3.5 {
		t.Fatalf("string %v %v", v, err)
	}
	if _, err := coerceFloat64("nope"); err == nil {
		t.Fatal("bad string")
	}
	if _, err := coerceFloat64([]byte{1}); err == nil {
		t.Fatal("bytes")
	}
}

func TestCoerceBool_FloatZero(t *testing.T) {
	if v, err := coerceBool(float64(0)); err != nil || v {
		t.Fatalf("0 → %v %v", v, err)
	}
}

func TestCoerceDateTime_NanoAndBadType(t *testing.T) {
	got, err := coerceDateTime("2026-08-06T12:00:00.123456789Z")
	if err != nil || got.Nanosecond() == 0 {
		t.Fatalf("nano: %v %v", got, err)
	}
}

func TestNormalizeValueType_PassthroughCanonical(t *testing.T) {
	for _, dt := range []ValueType{ValueBool, ValueInt64, ValueUint, ValueFloat64, ValueString, ValueDateTime} {
		if got := NormalizeValueType(dt); got != dt {
			t.Fatalf("%q → %q", dt, got)
		}
	}
	if got := NormalizeValueType("  TIMESTAMP "); got != ValueDateTime {
		t.Fatalf("timestamp alias: %q", got)
	}
	if got := NormalizeValueType("weird"); got != "weird" {
		t.Fatalf("unknown passthrough: %q", got)
	}
}


func TestCoerceString_Edges(t *testing.T) {
	if v, err := coerceString(float64(1.25)); err != nil || v != "1.25" {
		t.Fatalf("f64 %q %v", v, err)
	}
	if v, err := coerceString(true); err != nil || v != "true" {
		t.Fatalf("true %q %v", v, err)
	}
	if v, err := coerceString(false); err != nil || v != "false" {
		t.Fatalf("false %q %v", v, err)
	}
	if _, err := coerceString(int(1)); err == nil {
		t.Fatal("int")
	}
}

func TestCoerceDateTime_Edges(t *testing.T) {
	when := time.Date(2026, 8, 6, 12, 0, 0, 0, time.FixedZone("CET", 2*3600))
	got, err := coerceDateTime(when)
	if err != nil || !got.Equal(when.UTC()) {
		t.Fatalf("Time: %v %v", got, err)
	}
	got, err = coerceDateTime("2026-08-06T12:00:00Z")
	if err != nil || got.Year() != 2026 {
		t.Fatalf("RFC3339: %v %v", got, err)
	}
	if _, err := coerceDateTime(42); err == nil {
		t.Fatal("int")
	}
}
