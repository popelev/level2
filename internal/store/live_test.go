package store

import (
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
)

func TestLive_PollAvgMs(t *testing.T) {
	l := NewLive()
	n := 1.0
	l.Update(core.Sample{TagID: "t1", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})
	time.Sleep(20 * time.Millisecond)
	l.Update(core.Sample{TagID: "t1", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})
	time.Sleep(20 * time.Millisecond)
	l.Update(core.Sample{TagID: "t1", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})

	out := l.SnapshotDevices([]core.Device{{
		ID: "d",
		Tags: []core.Tag{{ID: "t1", NodeID: "ns=4;i=1", DataType: core.ValueFloat64}},
	}})
	if len(out) != 1 || out[0].PollAvgMs == nil || *out[0].PollAvgMs < 10 {
		t.Fatalf("got %#v", out[0].PollAvgMs)
	}
}

func TestLive_UpdateGetAndSnapshot(t *testing.T) {
	l := NewLive()
	if _, ok := l.Get("missing"); ok {
		t.Fatal("expected miss")
	}
	n := 42.0
	ts := time.Unix(10, 0).UTC()
	l.Update(core.Sample{TagID: "t1", Time: ts, ValueNum: &n, Quality: core.QualityGood})
	got, ok := l.Get("t1")
	if !ok || got.ValueNum == nil || *got.ValueNum != 42 || !got.Time.Equal(ts) {
		t.Fatalf("%#v ok=%v", got, ok)
	}

	out := l.SnapshotTags([]core.Tag{
		{ID: "t1", NodeID: "ns=4;i=1", DataType: core.ValueFloat64},
		{ID: "t2", NodeID: "ns=4;i=2", DataType: core.ValueFloat64},
	})
	if len(out) != 2 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0].Sample == nil || out[0].UpdatedAt == nil || !out[0].UpdatedAt.Equal(ts) {
		t.Fatalf("live tag: %#v", out[0])
	}
	if out[1].Sample != nil || out[1].PollAvgMs != nil {
		t.Fatalf("cold tag should have no sample: %#v", out[1])
	}
	if len(l.All()) != 1 {
		t.Fatalf("All=%d", len(l.All()))
	}
	if n := l.Clear(); n != 1 {
		t.Fatalf("Clear=%d", n)
	}
	if len(l.All()) != 0 {
		t.Fatalf("All after Clear=%d", len(l.All()))
	}
	if _, ok := l.Get("t1"); ok {
		t.Fatal("expected miss after Clear")
	}
	if n := l.Clear(); n != 0 {
		t.Fatalf("second Clear=%d", n)
	}
}

func TestLive_MarkQuality(t *testing.T) {
	l := NewLive()
	n := 3.5
	ts := time.Unix(10, 0).UTC()
	l.Update(core.Sample{TagID: "t1", Time: ts, ValueNum: &n, Quality: core.QualityGood})
	if got := l.MarkQuality([]string{"t1", "missing"}, core.QualityBad); len(got) != 1 {
		t.Fatalf("updated=%d", len(got))
	}
	s, ok := l.Get("t1")
	if !ok || s.Quality != core.QualityBad || s.ValueNum == nil || *s.ValueNum != 3.5 {
		t.Fatalf("want Bad keeping value: %#v", s)
	}
	if s.Time.Equal(ts) {
		t.Fatal("expected timestamp refresh on MarkQuality")
	}
	if got := l.MarkQuality([]string{"t1"}, core.QualityBad); len(got) != 0 {
		t.Fatalf("idempotent want 0 got %d", len(got))
	}
	if got := (*Live)(nil).MarkQuality([]string{"t1"}, core.QualityBad); got != nil {
		t.Fatalf("nil live: %#v", got)
	}
}

func TestLive_MarkQualityPreserveTime(t *testing.T) {
	l := NewLive()
	n := 3.5
	ts := time.Unix(10, 0).UTC()
	l.Update(core.Sample{TagID: "t1", Time: ts, ValueNum: &n, Quality: core.QualityGood})
	if got := l.MarkQualityPreserveTime([]string{"t1"}, core.QualityBad); len(got) != 1 {
		t.Fatalf("updated=%d", len(got))
	}
	s, ok := l.Get("t1")
	if !ok || s.Quality != core.QualityBad || !s.Time.Equal(ts) {
		t.Fatalf("want Bad keeping time: %#v", s)
	}
}

func TestAvgIntervals(t *testing.T) {
	if avgIntervals(nil) != 0 {
		t.Fatal("empty")
	}
	if avgIntervals([]int64{10, 20, 30}) != 20 {
		t.Fatal("avg")
	}
}

func TestLive_IntervalWindowCap(t *testing.T) {
	l := NewLive()
	n := 1.0
	// Force interval recording by updating with spaced wall clocks via lastRecv mutation under lock.
	l.Update(core.Sample{TagID: "t", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})
	l.mu.Lock()
	ent := l.byID["t"]
	ent.intervals = []int64{1, 2, 3, 4, 5}
	ent.lastRecv = time.Now().Add(-50 * time.Millisecond)
	l.mu.Unlock()
	time.Sleep(20 * time.Millisecond)
	l.Update(core.Sample{TagID: "t", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.byID["t"].intervals) != maxPollIntervals {
		t.Fatalf("want cap %d got %d (%v)", maxPollIntervals, len(l.byID["t"].intervals), l.byID["t"].intervals)
	}
}
