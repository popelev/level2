package simbrowser

import (
	"context"
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestExpandStructure_DefaultsAndDeep(t *testing.T) {
	b := NewDemo()
	ctx := context.Background()

	tags, err := b.ExpandStructure(ctx, "ns=4;i=4207", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 3 {
		t.Fatalf("want 3 leaves, got %d", len(tags))
	}
	for _, tg := range tags {
		if len(tg.ID) < 3 || tg.ID[:3] != "udt" {
			t.Fatalf("expected udt prefix, got %q", tg.ID)
		}
	}

	deep, err := b.ExpandStructure(ctx, "ns=2;i=5002", "tank", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(deep) < 4 {
		t.Fatalf("expected nested leaves, got %d %#v", len(deep), deep)
	}
}

func TestExpandStructure_MaxDepthAndWalkErrors(t *testing.T) {
	ctx := context.Background()
	b := NewDemo()

	// maxDepth=1 from Objects: only Server.ServerStatus is reachable as a leaf
	shallow, err := b.ExpandStructure(ctx, "ns=0;i=85", "root", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(shallow) != 1 || shallow[0].BrowsePath != "Server.ServerStatus" || shallow[0].NodeID != "ns=0;i=2256" {
		t.Fatalf("maxDepth=1: %#v", shallow)
	}

	if _, err := b.ExpandStructure(ctx, "ns=9;i=404", "x", 2); err == nil {
		t.Fatal("expected unknown node error")
	}

	// Folder child pointing at missing node → recursive walk error path
	broken := &Browser{children: map[string][]core.BrowseNode{
		"ns=1;i=1": {folder("ns=1;i=missing", "Gone")},
	}}
	if _, err := broken.ExpandStructure(ctx, "ns=1;i=1", "x", 4); err == nil {
		t.Fatal("expected walk error for missing child folder")
	}
}

func TestGuess_IntAndStringBranches(t *testing.T) {
	cases := []struct {
		name string
		want core.ValueType
	}{
		{"sUnit", core.ValueString},
		{"sName", core.ValueString},
		{"sText", core.ValueString},
		{"bValid", core.ValueBool},
		{"APM_AUTO", core.ValueBool},
		{"flag_bool", core.ValueBool},
		{"motor_run", core.ValueBool},
		{"iCount", core.ValueInt64},
		{"stack_count", core.ValueInt64},
		{"rValueOut", core.ValueFloat64},
	}
	for _, tc := range cases {
		if got := guess(tc.name); got != tc.want {
			t.Fatalf("guess(%q)=%q want %q", tc.name, got, tc.want)
		}
	}
}

func TestSanitize(t *testing.T) {
	got := sanitize("Tank-Data/A.B C")
	if got != "tank_data_a_b_c" {
		t.Fatalf("sanitize=%q", got)
	}
}
