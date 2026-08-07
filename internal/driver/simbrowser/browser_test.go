package simbrowser

import (
	"context"
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestDemoExpand(t *testing.T) {
	b := NewDemo()
	root, err := b.BrowseChildren(context.Background(), "ns=0;i=84")
	if err != nil || len(root) < 1 {
		t.Fatalf("root: %v %#v", err, root)
	}
	if root[0].BrowseName != "Objects" || root[0].IsLeaf {
		t.Fatalf("root child %#v", root[0])
	}
	tags, err := b.ExpandStructure(context.Background(), "ns=4;i=4207", "tank", 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 3 {
		t.Fatalf("want 3 leaves, got %d %#v", len(tags), tags)
	}
	byPath := map[string]core.ExpandedTag{}
	for _, tg := range tags {
		byPath[tg.BrowsePath] = tg
	}
	if byPath["rValueOut"].DataType != core.ValueFloat64 || byPath["rValueOut"].NodeID != "ns=4;i=4208" {
		t.Fatalf("%#v", byPath["rValueOut"])
	}
	if byPath["sUnit"].DataType != core.ValueString || byPath["bValid"].DataType != core.ValueBool {
		t.Fatalf("%#v", byPath)
	}
}
