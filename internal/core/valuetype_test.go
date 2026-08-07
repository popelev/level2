package core

import "testing"

func TestNormalizeValueType_DateTime(t *testing.T) {
	for _, in := range []string{"date_time", "DateTime", "datetime", "TIMESTAMP", "datetim"} {
		if got := NormalizeValueType(ValueType(in)); got != ValueDateTime {
			t.Fatalf("%q → %q", in, got)
		}
	}
	if !ValidValueType(ValueDateTime) {
		t.Fatal("datetime should be valid")
	}
}

func TestNormalizeValueType_Uint(t *testing.T) {
	for _, in := range []string{"uint", "uint32", "uint64", "byte", "UInt16"} {
		if got := NormalizeValueType(ValueType(in)); got != ValueUint {
			t.Fatalf("%q → %q, want uint", in, got)
		}
	}
	if !ValidValueType(ValueUint) {
		t.Fatal("uint should be valid")
	}
}
