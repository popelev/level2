package opcua

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

func TestWriteStatusError(t *testing.T) {
	var nilErr *WriteStatusError
	if got := nilErr.Error(); got != "opc write status error" {
		t.Fatalf("nil: %q", got)
	}
	e := &WriteStatusError{Status: ua.StatusBad}
	got := e.Error()
	if got == "" || got == "opc write status error" {
		t.Fatalf("non-nil Status.Error: %q", got)
	}
}

func TestCoerceWriteValue_UnsupportedAndAliases(t *testing.T) {
	if _, err := CoerceWriteValue("blob", 1); err == nil {
		t.Fatal("unsupported")
	}
	// Normalize aliases (e.g. "float" → float64) if core supports them; else still reject empty after normalize.
	got, err := CoerceWriteValue(core.ValueBool, float64(0))
	if err != nil || got != false {
		t.Fatalf("bool 0: %#v %v", got, err)
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
	if _, err := coerceFloat64([]byte{1}); err == nil {
		t.Fatal("bytes")
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

func TestWriteValue_NotConnected(t *testing.T) {
	d := New(core.Device{ID: "x"}, nil)
	err := d.WriteValue(context.Background(), core.Tag{ID: "t", NodeID: "ns=4;i=1"}, true)
	if err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("got %v", err)
	}
}

func TestReadValue_NotConnected(t *testing.T) {
	d := New(core.Device{ID: "x"}, nil)
	_, err := d.ReadValue(context.Background(), core.Tag{ID: "t", NodeID: "ns=4;i=1", DataType: core.ValueBool})
	if err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("got %v", err)
	}
}
