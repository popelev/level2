package core

import "testing"

func TestNormalizeValueType_Aliases(t *testing.T) {
	cases := map[string]ValueType{
		"boolean":      ValueBool,
		"BOOL":         ValueBool,
		"int16":        ValueInt64,
		"int":          ValueInt64,
		"integer":      ValueInt64,
		"float":        ValueFloat64,
		"double":       ValueFloat64,
		"string":       ValueString,
		"bytestring":   ValueString,
		"datetime":     ValueDateTime,
		"timestamp":    ValueDateTime,
		"datetim":      ValueDateTime,
		"utctime":      ValueDateTime,
		"localizedtext": ValueString,
	}
	for in, want := range cases {
		if got := NormalizeValueType(ValueType(in)); got != want {
			t.Fatalf("%q → %q want %q", in, got, want)
		}
	}
	if !ValidValueType("float64") || ValidValueType("struct") {
		t.Fatal("ValidValueType")
	}
	// unknown passthrough
	if got := NormalizeValueType("weird"); got != "weird" {
		t.Fatalf("passthrough %q", got)
	}
}
