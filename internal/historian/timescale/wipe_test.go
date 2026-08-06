package timescale

import (
	"context"
	"testing"
)

func TestWipeSamplesNilHistorian(t *testing.T) {
	var h *Historian
	_, err := h.WipeSamples(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	h2 := &Historian{}
	_, err = h2.WipeSamples(context.Background())
	if err == nil {
		t.Fatal("expected error for nil pool")
	}
}
