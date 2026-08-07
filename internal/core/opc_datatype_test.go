package core

import "testing"

func TestMapOPCTypeID_Builtins(t *testing.T) {
	cases := []struct {
		id   uint32
		want ValueType
	}{
		{1, ValueBool},
		{2, ValueInt64},
		{4, ValueInt64},
		{6, ValueInt64},
		{8, ValueInt64},
		{3, ValueUint},
		{5, ValueUint},
		{7, ValueUint},
		{9, ValueUint},
		{10, ValueFloat64},
		{11, ValueFloat64},
		{12, ValueString},
		{15, ValueString},
		{16, ValueString},
		{21, ValueString},
		{13, ValueDateTime},
		{294, ValueDateTime},
		{99999, ""},
	}
	for _, tc := range cases {
		if got := MapOPCTypeID(tc.id); got != tc.want {
			t.Fatalf("id=%d got %q want %q", tc.id, got, tc.want)
		}
	}
}

func TestMapOPCTypeCode_Formats(t *testing.T) {
	cases := []struct {
		code string
		want ValueType
	}{
		{"i=1", ValueBool},
		{"i=4", ValueInt64},
		{"i=7", ValueUint},
		{"i=10", ValueFloat64},
		{"i=12", ValueString},
		{"i=13", ValueDateTime},
		{"ns=0;i=10", ValueFloat64},
		{"10", ValueFloat64},
		{"i=294", ValueDateTime},
		{"i=15", ValueString},
		{"", ""},
		{"nope", ""},
		{"i=99999", ""},
	}
	for _, tc := range cases {
		if got := MapOPCTypeCode(tc.code); got != tc.want {
			t.Fatalf("code %q → %q want %q", tc.code, got, tc.want)
		}
	}
}

func TestMapOPCTypeName_AndOPCTypeCode(t *testing.T) {
	if got := MapOPCTypeName("Float"); got != ValueFloat64 {
		t.Fatalf("Float → %q", got)
	}
	if got := MapOPCTypeName("ByteString"); got != ValueString {
		t.Fatalf("ByteString → %q", got)
	}
	if got := MapOPCTypeName("UtcTime"); got != ValueDateTime {
		t.Fatalf("UtcTime → %q", got)
	}
	if got := MapOPCTypeName("integer"); got != ValueInt64 {
		t.Fatalf("integer → %q", got)
	}
	if got := MapOPCTypeName(""); got != "" {
		t.Fatal("empty name")
	}
	if got := MapOPCTypeName("unknown"); got != "" {
		t.Fatalf("unknown → %q", got)
	}

	if OPCTypeCode(ValueBool) != "i=1" || OPCTypeCode(ValueFloat64) != "i=10" {
		t.Fatal("OPCTypeCode canonical")
	}
	if OPCTypeCode("float") != "i=10" {
		t.Fatal("OPCTypeCode alias")
	}
}
