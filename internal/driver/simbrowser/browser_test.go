package simbrowser

import (
	"context"
	"testing"
)

func TestDemoExpand(t *testing.T) {
	b := NewDemo()
	tags, err := b.ExpandStructure(context.Background(), "ns=4;i=4207", "tank", 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 3 {
		t.Fatalf("want 3 leaves, got %d %#v", len(tags), tags)
	}
	ids := map[string]bool{}
	for _, tg := range tags {
		ids[tg.ID] = true
	}
	for _, want := range []string{"tank_rvalueout", "tank_sunit", "tank_bvalid"} {
		if !ids[want] {
			t.Fatalf("missing %s in %#v", want, tags)
		}
	}
}
