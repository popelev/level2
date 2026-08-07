package core

import "testing"

func TestNormalizeValueType_CanonicalAndWhitespace(t *testing.T) {
	t.Parallel()
	for _, dt := range []ValueType{ValueBool, ValueInt64, ValueUint, ValueFloat64, ValueString, ValueDateTime} {
		if got := NormalizeValueType(dt); got != dt {
			t.Fatalf("%q → %q", dt, got)
		}
	}
	if got := NormalizeValueType("  float64  "); got != ValueFloat64 {
		t.Fatalf("padded float64 → %q", got)
	}
	if got := NormalizeValueType("Byte"); got != ValueUint {
		t.Fatalf("Byte → %q", got)
	}
	if ValidValueType("") || ValidValueType("json") {
		t.Fatal("invalid types")
	}
}

func TestSamePayload_NilPointers(t *testing.T) {
	t.Parallel()
	a := Sample{TagID: "t", Quality: QualityGood}
	b := Sample{TagID: "t", Quality: QualityGood}
	if !a.SamePayload(b) {
		t.Fatal("empty equal")
	}
	n := 1.0
	a.ValueNum = &n
	if a.SamePayload(b) {
		t.Fatal("num vs nil")
	}
	txt := "x"
	b.ValueNum = &n
	a.ValueText = &txt
	if a.SamePayload(b) {
		t.Fatal("text vs nil text")
	}
}
