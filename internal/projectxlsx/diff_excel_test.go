package projectxlsx

import (
	"strings"
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestDiffExcel_RoundTrip(t *testing.T) {
	a := []core.Device{{
		ID: "s1", Endpoint: "opc.tcp://a", Security: "None",
		Tags: []core.Tag{{ID: "t1", NodeID: "ns=1;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000}},
	}}
	b := []core.Device{{
		ID: "s1", Endpoint: "opc.tcp://b", Security: "None",
		Tags: []core.Tag{{ID: "t1", NodeID: "ns=1;i=2", DataType: core.ValueBool, Enabled: false, IntervalMs: 500}},
	}}
	rows := Compare(a, b)
	srv, tags := DiffToSheets(rows)
	raw, err := DiffExcel(srv, tags)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 100 {
		t.Fatalf("len=%d", len(raw))
	}
}

func TestParseBoolAndCellAt(t *testing.T) {
	if !parseBool("YES", false) || parseBool("0", true) || !parseBool("", true) || parseBool("maybe", false) {
		t.Fatal("parseBool cases")
	}
	if cellAt(nil, 0) != "" || cellAt([]string{" a "}, 0) != "a" || cellAt([]string{"a"}, 2) != "" {
		t.Fatal("cellAt")
	}
}

func TestFormatErr(t *testing.T) {
	if FormatErr(nil, 1) != "" {
		t.Fatal("empty")
	}
	if FormatErr([]string{"one"}, 1) != "one" {
		t.Fatal("single")
	}
	got := FormatErr([]string{"a", "b", "c"}, 1)
	if !strings.Contains(got, "a") || !strings.Contains(got, "+2") {
		t.Fatalf("%q", got)
	}
}
