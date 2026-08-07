package importexcel

import (
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestMapDataType_NamesAndCodes(t *testing.T) {
	cases := []struct {
		name, code string
		want       core.ValueType
	}{
		{"Float", "", core.ValueFloat64},
		{"Boolean", "", core.ValueBool},
		{"String", "", core.ValueString},
		{"DateTime", "", core.ValueDateTime},
		{"Int16", "", core.ValueInt64},
		{"UInt32", "", core.ValueUint},
		{"", "i=1", core.ValueBool},
		{"", "i=4", core.ValueInt64},
		{"", "i=7", core.ValueUint},
		{"", "i=10", core.ValueFloat64},
		{"", "i=12", core.ValueString},
		{"", "i=13", core.ValueDateTime},
		{"unknown", "i=10", core.ValueFloat64},
	}
	for _, tc := range cases {
		got := mapDataType(tc.name, tc.code)
		if got != tc.want {
			t.Fatalf("mapDataType(%q,%q)=%q want %q", tc.name, tc.code, got, tc.want)
		}
	}
}

func TestCellAndJoinPath(t *testing.T) {
	if cell(nil, 0) != "" || cell([]string{" x "}, 0) != "x" {
		t.Fatal("cell")
	}
	if joinPath("A", "", "B/C") != "A/B/C" {
		t.Fatalf("joinPath=%q", joinPath("A", "", "B/C"))
	}
	if sanitizeID(" a b ") != "a_b" {
		t.Fatalf("sanitize=%q", sanitizeID(" a b "))
	}
}
