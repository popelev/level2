package timescale

import (
	"context"
	"testing"
)

func TestResolveCapacityLimit_EnvAndDisk(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "12345")
	limit, total := resolveCapacityLimit(50)
	if limit != 12345 || total != 12345 {
		t.Fatalf("env override limit=%d total=%d", limit, total)
	}

	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "")
	dir := t.TempDir()
	t.Setenv("LEVEL2_DB_DATA_PATH", dir)
	limit, total = resolveCapacityLimit(0) // → 90%
	if limit <= 0 || total <= 0 || limit > total {
		t.Fatalf("disk limit=%d total=%d", limit, total)
	}
	limit100, total100 := resolveCapacityLimit(200) // clamp to 100
	if limit100 != total100 || total100 <= 0 {
		t.Fatalf("100%% limit=%d total=%d", limit100, total100)
	}
}

func TestEnforceCapacity_NilSafe(t *testing.T) {
	var h *Historian
	if err := h.enforceCapacity(context.Background()); err != nil {
		t.Fatal(err)
	}
	h2 := &Historian{}
	if err := h2.enforceCapacity(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCapacityPolicy_NilDefaults(t *testing.T) {
	var h *Historian
	got := h.CapacityPolicy()
	if got.Percent != 90 || got.Policy == "" {
		t.Fatalf("%+v", got)
	}
}
