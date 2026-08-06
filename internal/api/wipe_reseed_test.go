package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/store"
)

func TestReseedAfterWipe_ClearsLiveAndWritesBatch(t *testing.T) {
	live := store.NewLive()
	n := 7.0
	live.Update(core.Sample{TagID: "a", Time: time.Unix(1, 0).UTC(), ValueNum: &n, Quality: core.QualityGood})
	live.Update(core.Sample{TagID: "b", Time: time.Unix(2, 0).UTC(), ValueNum: &n, Quality: core.QualityGood})

	var got []core.Sample
	cleared, written, err := reseedAfterWipe(context.Background(), live, func(_ context.Context, samples []core.Sample) error {
		got = append([]core.Sample(nil), samples...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if cleared != 2 || written != 2 {
		t.Fatalf("cleared=%d written=%d", cleared, written)
	}
	if len(live.All()) != 0 {
		t.Fatalf("live should be empty, got %d", len(live.All()))
	}
	if len(got) != 2 {
		t.Fatalf("batch len=%d", len(got))
	}
	for _, s := range got {
		if s.Time.Equal(time.Unix(1, 0).UTC()) || s.Time.Equal(time.Unix(2, 0).UTC()) {
			t.Fatalf("expected fresh timestamps, got %#v", s)
		}
	}
}

func TestReseedAfterWipe_WriteErrorStillClearsLive(t *testing.T) {
	live := store.NewLive()
	n := 1.0
	live.Update(core.Sample{TagID: "a", Time: time.Unix(1, 0).UTC(), ValueNum: &n, Quality: core.QualityGood})

	cleared, written, err := reseedAfterWipe(context.Background(), live, func(context.Context, []core.Sample) error {
		return errors.New("db down")
	})
	if err == nil {
		t.Fatal("expected write error")
	}
	if cleared != 1 || written != 0 {
		t.Fatalf("cleared=%d written=%d", cleared, written)
	}
	if len(live.All()) != 0 {
		t.Fatal("live must stay cleared after write failure")
	}
}

func TestReseedAfterWipe_NilLive(t *testing.T) {
	cleared, written, err := reseedAfterWipe(context.Background(), nil, nil)
	if err != nil || cleared != 0 || written != 0 {
		t.Fatalf("cleared=%d written=%d err=%v", cleared, written, err)
	}
}

func TestFanIn_AfterWipeClear_DoesNotSuppress(t *testing.T) {
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
	first := core.Sample{Time: time.Unix(1, 0).UTC(), TagID: "tag.a", ValueNum: &n, Quality: core.QualityGood}
	in <- first
	_ = recvSample(t, out, 500*time.Millisecond)

	// Identical poll would suppress — until wipe clears Live + reseeds.
	nSame := 10.0
	in <- core.Sample{Time: time.Unix(2, 0).UTC(), TagID: "tag.a", ValueNum: &nSame, Quality: core.QualityGood}
	assertNoSample(t, out, 80*time.Millisecond)

	cleared, written, err := reseedAfterWipe(ctx, live, func(context.Context, []core.Sample) error {
		return nil // WriteBatch succeeded (historian re-seeded)
	})
	if err != nil || cleared != 1 || written != 1 {
		t.Fatalf("reseed cleared=%d written=%d err=%v", cleared, written, err)
	}

	// Same payload after wipe must reach historian again (no Live prev).
	in <- core.Sample{Time: time.Unix(3, 0).UTC(), TagID: "tag.a", ValueNum: &nSame, Quality: core.QualityGood}
	got := recvSample(t, out, 500*time.Millisecond)
	if got.TagID != "tag.a" || got.ValueNum == nil || *got.ValueNum != 10 {
		t.Fatalf("post-wipe sample suppressed: %#v", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("FanIn did not exit")
	}
}
