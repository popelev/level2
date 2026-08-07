package timescale

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/popelev/level2/internal/config"
)

func TestResolveCapacityLimitEnv(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "5000")
	t.Setenv("LEVEL2_DB_DATA_PATH", t.TempDir())
	limit, total := resolveCapacityLimit(50)
	if limit != 5000 || total != 5000 {
		t.Fatalf("limit=%d total=%d", limit, total)
	}
}

func TestResolveCapacityLimitPercent(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "")
	dir := t.TempDir()
	t.Setenv("LEVEL2_DB_DATA_PATH", dir)
	limit, total := resolveCapacityLimit(25)
	if total <= 0 {
		t.Fatalf("total=%d", total)
	}
	want := total * 25 / 100
	if limit != want {
		t.Fatalf("limit=%d want=%d total=%d", limit, want, total)
	}
}

func TestCapacityPolicyDefaults(t *testing.T) {
	h := &Historian{}
	h.SetCapacityPolicy(0, "")
	got := h.CapacityPolicy()
	if got.Percent != 90 || got.Policy != config.FullPolicyStop {
		t.Fatalf("%+v", got)
	}
	h.SetCapacityPolicy(55, config.FullPolicyDropOldest)
	got = h.CapacityPolicy()
	if got.Percent != 55 || got.Policy != config.FullPolicyDropOldest {
		t.Fatalf("%+v", got)
	}
}

func TestErrCapacityHaltIs(t *testing.T) {
	w := fmt.Errorf("%w: stop policy", ErrCapacityHalt)
	if !errors.Is(w, ErrCapacityHalt) {
		t.Fatalf("expected Is ErrCapacityHalt: %v", w)
	}
}

func TestErrCapacityBusyIs(t *testing.T) {
	w := fmt.Errorf("%w: still over limit after drop_oldest", ErrCapacityBusy)
	if !errors.Is(w, ErrCapacityBusy) {
		t.Fatalf("expected Is ErrCapacityBusy: %v", w)
	}
	if errors.Is(w, ErrCapacityHalt) {
		t.Fatal("busy must not be halt")
	}
}

func TestPlanChunkDrop_ProportionalToOverrun(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	chunks := make([]chunkSizeRow, 10)
	for i := range chunks {
		chunks[i] = chunkSizeRow{rangeStart: base.Add(time.Duration(i) * time.Hour), sizeBytes: 100}
	}
	n, cutoff, freed, ok := planChunkDrop(chunks, 250, 1000, 64)
	if !ok || n != 3 || freed != 300 {
		t.Fatalf("n=%d freed=%d ok=%v", n, freed, ok)
	}
	if !cutoff.Equal(base.Add(3 * time.Hour)) {
		t.Fatalf("cutoff=%v", cutoff)
	}
}

func TestPlanChunkDrop_UnknownSizesUsesAvg(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	chunks := make([]chunkSizeRow, 5)
	for i := range chunks {
		chunks[i] = chunkSizeRow{rangeStart: base.Add(time.Duration(i) * time.Hour), sizeBytes: 0}
	}
	// dbUsed=500 → avg=100; need=350 → drop 4 (keep 1)
	n, _, freed, ok := planChunkDrop(chunks, 350, 500, 64)
	if !ok || n != 4 || freed != 400 {
		t.Fatalf("n=%d freed=%d ok=%v", n, freed, ok)
	}
}

func TestPlanChunkDrop_KeepsNewest(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	chunks := []chunkSizeRow{
		{rangeStart: base, sizeBytes: 1000},
		{rangeStart: base.Add(time.Hour), sizeBytes: 1000},
	}
	n, cutoff, freed, ok := planChunkDrop(chunks, 9999, 2000, 64)
	if !ok || n != 1 || freed != 1000 || !cutoff.Equal(base.Add(time.Hour)) {
		t.Fatalf("n=%d freed=%d cutoff=%v ok=%v", n, freed, cutoff, ok)
	}
}

func TestPlanChunkDrop_Empty(t *testing.T) {
	if _, _, _, ok := planChunkDrop(nil, 100, 100, 8); ok {
		t.Fatal("expected !ok")
	}
}
