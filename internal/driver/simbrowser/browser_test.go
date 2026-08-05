package simbrowser

import (
	"context"
	"testing"
)

func TestDemoExpand(t *testing.T) {
	b := NewDemo()
	root, err := b.BrowseChildren(context.Background(), "ns=0;i=84")
	if err != nil || len(root) < 1 {
		t.Fatalf("root: %v %#v", err, root)
	}
	tags, err := b.ExpandStructure(context.Background(), "ns=4;i=4207", "tank", 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 3 {
		t.Fatalf("want 3 leaves, got %d %#v", len(tags), tags)
	}
}
