package simbrowser

import (
	"context"
	"testing"
)

func TestProbeNode(t *testing.T) {
	b := NewDemo()
	ctx := context.Background()

	if _, _, err := b.ProbeNode(ctx, ""); err == nil {
		t.Fatal("empty node id")
	}
	exists, isLeaf, err := b.ProbeNode(ctx, "ns=4;i=4207")
	if err != nil || !exists || isLeaf {
		t.Fatalf("folder: exists=%v leaf=%v err=%v", exists, isLeaf, err)
	}
	exists, isLeaf, err = b.ProbeNode(ctx, "ns=4;i=4208")
	if err != nil || !exists || !isLeaf {
		t.Fatalf("leaf: exists=%v leaf=%v err=%v", exists, isLeaf, err)
	}
	exists, isLeaf, err = b.ProbeNode(ctx, "ns=4;i=999999")
	if err != nil || exists || isLeaf {
		t.Fatalf("missing: exists=%v leaf=%v err=%v", exists, isLeaf, err)
	}
}

func TestBrowseChildren_Unknown(t *testing.T) {
	b := NewDemo()
	if _, err := b.BrowseChildren(context.Background(), "ns=9;i=1"); err == nil {
		t.Fatal("expected unknown node")
	}
	root, err := b.BrowseChildren(context.Background(), "")
	if err != nil || len(root) == 0 {
		t.Fatalf("default root: %v %#v", err, root)
	}
}
