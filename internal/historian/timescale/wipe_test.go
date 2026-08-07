package timescale

import (
	"context"
	"testing"
)

func TestWipeSamplesNilHistorian(t *testing.T) {
	var h *Historian
	out, err := h.WipeSamples(context.Background())
	if err == nil || err.Error() != "historian not configured" {
		t.Fatalf("nil historian: out=%+v err=%v", out, err)
	}
	if out.Method != "" || out.ApproxRows != 0 {
		t.Fatalf("zero result on error: %+v", out)
	}
	h2 := &Historian{}
	out, err = h2.WipeSamples(context.Background())
	if err == nil || err.Error() != "historian not configured" {
		t.Fatalf("nil pool: out=%+v err=%v", out, err)
	}
}
