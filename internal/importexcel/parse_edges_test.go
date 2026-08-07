package importexcel

import (
	"bytes"
	"strings"
	"testing"

	"github.com/popelev/level2/internal/core"
	"github.com/xuri/excelize/v2"
)

func TestParse_ErrorPaths(t *testing.T) {
	if _, err := Parse(strings.NewReader("not-xlsx")); err == nil {
		t.Fatal("expected open xlsx error")
	}

	// Empty sheet (header only missing → <2 rows)
	empty := mustXLSX(t, []string{"Area", "Path", "Signal", "MeasurePoint NodeId", "DataType", "DataType Name"}, nil)
	if _, err := Parse(bytes.NewReader(empty)); err == nil {
		t.Fatal("expected empty sheet")
	}

	// Missing required columns
	badHdr := mustXLSX(t, []string{"Area", "Path", "Nope"}, [][]any{{"a", "b", "c"}})
	if _, err := Parse(bytes.NewReader(badHdr)); err == nil {
		t.Fatal("expected missing columns")
	}

	// Blank row skipped; empty signal; empty datatype; duplicate
	rows := [][]any{
		{"", "", "", "", "", ""},                       // blank skip
		{"A", "P", "   ", "ns=1;i=1", "i=10", "Float"}, // empty signal
		{"A", "P", "NODT", "ns=1;i=2", "", ""},         // unsupported (empty type → "")
		{"A", "P", "DUP", "ns=1;i=3", "i=10", "Float"}, // ok
		{"A", "P", "DUP", "ns=1;i=4", "i=10", "Float"}, // duplicate
	}
	raw := mustXLSX(t, []string{"Area", "Path", "Signal", "MeasurePoint NodeId", "DataType", "DataType Name"}, rows)
	res, err := Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tags) != 1 || res.Tags[0].ID != "DUP" {
		t.Fatalf("tags=%#v errors=%v", res.Tags, res.Errors)
	}
	if len(res.Errors) < 3 {
		t.Fatalf("want errors for empty/bad/dup, got %v", res.Errors)
	}

	// All rows invalid → no valid tags
	onlyBad := mustXLSX(t, []string{"Area", "Path", "Signal", "MeasurePoint NodeId", "DataType", "DataType Name"},
		[][]any{{"A", "P", "X", "not-a-node", "i=10", "Float"}})
	if _, err := Parse(bytes.NewReader(onlyBad)); err == nil {
		t.Fatal("expected no valid tags")
	}
}

func TestParse_NoSheets(t *testing.T) {
	f := excelize.NewFile()
	for _, name := range f.GetSheetList() {
		_ = f.DeleteSheet(name)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(&buf); err == nil {
		t.Fatal("expected no sheets error")
	}
}

func TestParse_AlternateNodeIDHeader(t *testing.T) {
	raw := mustXLSX(t, []string{"Signal", "NodeId", "DataType"},
		[][]any{{"ALT", "ns=2;i=9", "i=12"}})
	res, err := Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tags) != 1 || res.Tags[0].DataType != core.ValueString {
		t.Fatalf("%#v", res.Tags)
	}
}

func TestMapDataType_MoreAliasesAndFallback(t *testing.T) {
	cases := []struct {
		name, code string
		want       core.ValueType
	}{
		{"double", "", core.ValueFloat64},
		{"float32", "", core.ValueFloat64},
		{"bool", "", core.ValueBool},
		{"bytestring", "", core.ValueString},
		{"date_time", "", core.ValueDateTime},
		{"timestamp", "", core.ValueDateTime},
		{"sbyte", "", core.ValueInt64},
		{"integer", "", core.ValueInt64},
		{"byte", "", core.ValueUint},
		{"unsigned", "", core.ValueUint},
		{"", "i=2", core.ValueInt64},
		{"", "i=3", core.ValueUint},
		{"", "i=5", core.ValueUint},
		{"", "i=6", core.ValueInt64},
		{"", "i=8", core.ValueInt64},
		{"", "i=9", core.ValueUint},
		{"", "i=11", core.ValueFloat64},
		{"float64", "nope", core.ValueFloat64}, // NormalizeValueType fallback
	}
	for _, tc := range cases {
		got := mapDataType(tc.name, tc.code)
		if got != tc.want {
			t.Fatalf("mapDataType(%q,%q)=%q want %q", tc.name, tc.code, got, tc.want)
		}
	}
	if got := mapDataType("totally_unknown", "zzz"); got != "totally_unknown" {
		t.Fatalf("unknown fallback: got %q want totally_unknown", got)
	}
	if got := mapDataType("", ""); got != "" {
		t.Fatalf("empty name+code should stay empty, got %q", got)
	}
}

func mustXLSX(t *testing.T, headers []string, rows [][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			t.Fatal(err)
		}
	}
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
