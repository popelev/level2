package timescale

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/popelev/level2/internal/config"
)

func TestCapacityPolicy_NilHistorian(t *testing.T) {
	var h *Historian
	h.SetCapacityPolicy(50, config.FullPolicyStop) // no-op
	got := h.CapacityPolicy()
	if got.Percent != 90 || got.Policy != config.FullPolicyStop {
		t.Fatalf("%+v", got)
	}
	if err := h.enforceCapacity(context.Background()); err != nil {
		t.Fatalf("nil enforce: %v", err)
	}
}

func TestCapacityPolicy_NoPoolAllowsWrite(t *testing.T) {
	h := &Historian{}
	h.SetCapacityPolicy(90, config.FullPolicyStop)
	if err := h.enforceCapacity(context.Background()); err != nil {
		t.Fatalf("no pool should allow: %v", err)
	}
}

func TestCapacityPolicy_DefaultsOnEmptyAtomic(t *testing.T) {
	h := &Historian{}
	got := h.CapacityPolicy()
	if got.Percent != 90 || got.Policy != config.FullPolicyStop {
		t.Fatalf("%+v", got)
	}
	h.SetCapacityPolicy(200, config.FullPolicyRotate)
	got = h.CapacityPolicy()
	if got.Percent != 100 || got.Policy != config.FullPolicyRotate {
		t.Fatalf("clamp %+v", got)
	}
}

func TestPoolNil(t *testing.T) {
	var h *Historian
	if h.Pool() != nil {
		t.Fatal("nil historian pool")
	}
	if (&Historian{}).Pool() != nil {
		t.Fatal("empty historian pool")
	}
}

func TestErrCapacityHaltWrapped(t *testing.T) {
	plain := errors.New("x")
	if errors.Is(plain, ErrCapacityHalt) {
		t.Fatal("plain error must not match")
	}
	wrapped := fmt.Errorf("%w: stop policy", ErrCapacityHalt)
	if !errors.Is(wrapped, ErrCapacityHalt) {
		t.Fatalf("wrapped halt: %v", wrapped)
	}
	if !errors.Is(fmt.Errorf("%w: still over limit after drop_oldest", ErrCapacityHalt), ErrCapacityHalt) {
		t.Fatal("drop_oldest wrap")
	}
}
