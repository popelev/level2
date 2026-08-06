package api

import (
	"context"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/store"
)

func TestFanIn_StringAndBoolEdges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	in := make(chan core.Sample, 8)
	out := make(chan core.Sample, 8)
	live := store.NewLive()
	hub := NewHub()

	done := make(chan struct{})
	go func() {
		FanIn(ctx, in, live, hub, out)
		close(done)
	}()

	txt := "barg"
	t0 := time.Unix(10, 0).UTC()
	in <- core.Sample{Time: t0, TagID: "unit", ValueText: &txt, Quality: core.QualityGood}
	got := recvSample(t, out, 500*time.Millisecond)
	if got.ValueText == nil || *got.ValueText != "barg" {
		t.Fatalf("first text: %#v", got)
	}

	// Same text → suppress.
	txt2 := "barg"
	in <- core.Sample{Time: time.Unix(11, 0).UTC(), TagID: "unit", ValueText: &txt2, Quality: core.QualityGood}
	assertNoSample(t, out, 80*time.Millisecond)

	// Text change → write.
	txt3 := "kPa"
	in <- core.Sample{Time: time.Unix(12, 0).UTC(), TagID: "unit", ValueText: &txt3, Quality: core.QualityGood}
	got = recvSample(t, out, 500*time.Millisecond)
	if got.ValueText == nil || *got.ValueText != "kPa" {
		t.Fatalf("text change: %#v", got)
	}

	b := true
	in <- core.Sample{Time: time.Unix(20, 0).UTC(), TagID: "en", ValueBool: &b, Quality: core.QualityGood}
	got = recvSample(t, out, 500*time.Millisecond)
	if got.ValueBool == nil || !*got.ValueBool {
		t.Fatalf("bool first: %#v", got)
	}
	bSame := true
	in <- core.Sample{Time: time.Unix(21, 0).UTC(), TagID: "en", ValueBool: &bSame, Quality: core.QualityGood}
	assertNoSample(t, out, 80*time.Millisecond)

	// Independent tags: suppress on unit must not affect en flips.
	bFalse := false
	in <- core.Sample{Time: time.Unix(22, 0).UTC(), TagID: "en", ValueBool: &bFalse, Quality: core.QualityGood}
	got = recvSample(t, out, 500*time.Millisecond)
	if got.TagID != "en" || got.ValueBool == nil || *got.ValueBool {
		t.Fatalf("bool flip: %#v", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("FanIn did not exit")
	}
}

func TestFanIn_ClosedInput(t *testing.T) {
	ctx := context.Background()
	in := make(chan core.Sample)
	out := make(chan core.Sample, 1)
	live := store.NewLive()
	hub := NewHub()

	done := make(chan struct{})
	go func() {
		FanIn(ctx, in, live, hub, out)
		close(done)
	}()
	close(in)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("FanIn did not exit on closed in")
	}
}

func TestFanIn_FirstSampleNoPrev(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan core.Sample, 1)
	out := make(chan core.Sample, 1)
	live := store.NewLive()
	hub := NewHub()
	go FanIn(ctx, in, live, hub, out)

	n := 0.0
	in <- core.Sample{Time: time.Unix(1, 0).UTC(), TagID: "fresh", ValueNum: &n, Quality: core.QualityGood}
	got := recvSample(t, out, 500*time.Millisecond)
	if got.TagID != "fresh" {
		t.Fatalf("%#v", got)
	}
	cancel()
}
