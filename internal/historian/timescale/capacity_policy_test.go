package timescale

import (
	"errors"
	"fmt"
	"testing"

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
