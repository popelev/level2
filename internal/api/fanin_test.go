package api

import (
	"context"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/store"
)

func TestFanIn_SuppressUnchanged(t *testing.T) {
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

	n := 10.0
	t0 := time.Unix(1, 0).UTC()
	first := core.Sample{Time: t0, TagID: "tag.a", ValueNum: &n, Quality: core.QualityGood}
	in <- first
	got := recvSample(t, out, 500*time.Millisecond)
	if got.TagID != "tag.a" || got.ValueNum == nil || *got.ValueNum != 10 {
		t.Fatalf("first sample: %#v", got)
	}
	if lv, ok := live.Get("tag.a"); !ok || lv.ValueNum == nil || *lv.ValueNum != 10 {
		t.Fatalf("live after first: ok=%v %#v", ok, lv)
	}

	// Identical payload, different time — suppress historian, still update Live.
	nSame := 10.0
	t1 := time.Unix(2, 0).UTC()
	in <- core.Sample{Time: t1, TagID: "tag.a", ValueNum: &nSame, Quality: core.QualityGood}
	assertNoSample(t, out, 80*time.Millisecond)
	if lv, ok := live.Get("tag.a"); !ok || !lv.Time.Equal(t1) {
		t.Fatalf("live time should update on suppress: ok=%v %#v", ok, lv)
	}

	// Value change → write.
	n2 := 11.0
	t2 := time.Unix(3, 0).UTC()
	in <- core.Sample{Time: t2, TagID: "tag.a", ValueNum: &n2, Quality: core.QualityGood}
	got = recvSample(t, out, 500*time.Millisecond)
	if got.ValueNum == nil || *got.ValueNum != 11 {
		t.Fatalf("value change: %#v", got)
	}

	// Quality-only change → write.
	n3 := 11.0
	t3 := time.Unix(4, 0).UTC()
	in <- core.Sample{Time: t3, TagID: "tag.a", ValueNum: &n3, Quality: core.QualityBad}
	got = recvSample(t, out, 500*time.Millisecond)
	if got.Quality != core.QualityBad {
		t.Fatalf("quality change: %#v", got)
	}

	// Another identical (bad/11) → suppress.
	n4 := 11.0
	in <- core.Sample{Time: time.Unix(5, 0).UTC(), TagID: "tag.a", ValueNum: &n4, Quality: core.QualityBad}
	assertNoSample(t, out, 80*time.Millisecond)

	// Bare Bad (no values) after Good — keep last value, flip quality (link loss path).
	in <- core.Sample{Time: time.Unix(6, 0).UTC(), TagID: "tag.a", Quality: core.QualityGood, ValueNum: &n2}
	got = recvSample(t, out, 500*time.Millisecond)
	if got.Quality != core.QualityGood {
		t.Fatalf("restore good: %#v", got)
	}
	tBare := time.Unix(7, 0).UTC()
	in <- core.Sample{Time: tBare, TagID: "tag.a", Quality: core.QualityBad}
	got = recvSample(t, out, 500*time.Millisecond)
	if got.Quality != core.QualityBad || got.ValueNum == nil || *got.ValueNum != 11 {
		t.Fatalf("bare Bad should keep last value: %#v", got)
	}
	if lv, ok := live.Get("tag.a"); !ok || lv.ValueNum == nil || *lv.ValueNum != 11 || lv.Quality != core.QualityBad {
		t.Fatalf("live after bare Bad: ok=%v %#v", ok, lv)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("FanIn did not exit")
	}
}

func recvSample(t *testing.T, out <-chan core.Sample, wait time.Duration) core.Sample {
	t.Helper()
	select {
	case s := <-out:
		return s
	case <-time.After(wait):
		t.Fatal("timeout waiting for historian sample")
		return core.Sample{}
	}
}

func assertNoSample(t *testing.T, out <-chan core.Sample, wait time.Duration) {
	t.Helper()
	select {
	case s := <-out:
		t.Fatalf("unexpected historian sample: %#v", s)
	case <-time.After(wait):
	}
}
