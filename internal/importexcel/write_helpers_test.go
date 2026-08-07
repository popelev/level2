package importexcel

import (
	"bytes"
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestWrite_AllDataTypesAndPaths(t *testing.T) {
	tags := []core.Tag{
		{ID: "f", NodeID: "ns=1;i=1", Path: "Area/Path/Leaf", DataType: core.ValueFloat64},
		{ID: "b", NodeID: "ns=1;i=2", Path: "SoloArea", DataType: core.ValueBool},
		{ID: "i", NodeID: "ns=1;i=3", Path: "", DataType: core.ValueInt64},
		{ID: "u", NodeID: "ns=1;i=4", Path: "/trim/me/", DataType: core.ValueUint},
		{ID: "s", NodeID: "ns=1;i=5", DataType: core.ValueString},
		{ID: "dt", NodeID: "ns=1;i=6", DataType: core.ValueDateTime},
	}
	raw, err := Write(tags)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tags) != len(tags) {
		t.Fatalf("got %d tags errors=%v", len(res.Tags), res.Errors)
	}
	byID := map[string]core.Tag{}
	for _, tg := range res.Tags {
		byID[tg.ID] = tg
	}
	if byID["b"].DataType != core.ValueBool || byID["i"].DataType != core.ValueInt64 {
		t.Fatalf("%#v", byID)
	}
	if byID["u"].DataType != core.ValueUint || byID["s"].DataType != core.ValueString {
		t.Fatalf("%#v", byID)
	}
	if byID["dt"].DataType != core.ValueDateTime {
		t.Fatalf("datetime %#v", byID["dt"])
	}
}

func TestSplitStoredPathAndTypeHelpers(t *testing.T) {
	cases := []struct {
		in         string
		wantArea   string
		wantPath   string
	}{
		{"", "", ""},
		{"Area", "Area", ""},
		{"Area/Path", "Area", "Path"},
		{"/A/B/C/", "A", "B/C"},
	}
	for _, tc := range cases {
		a, p := splitStoredPath(tc.in)
		if a != tc.wantArea || p != tc.wantPath {
			t.Fatalf("%q → (%q,%q) want (%q,%q)", tc.in, a, p, tc.wantArea, tc.wantPath)
		}
	}

	types := []core.ValueType{
		core.ValueBool, core.ValueInt64, core.ValueUint, core.ValueString, core.ValueDateTime, core.ValueFloat64,
	}
	for _, dt := range types {
		if typeDisplayName(dt) == "" || core.OPCTypeCode(dt) == "" {
			t.Fatalf("helpers empty for %q", dt)
		}
	}
}
