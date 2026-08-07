package timescale

import (
	"context"
	"testing"

	"github.com/popelev/level2/internal/core"
)

// Empty batches must short-circuit before pool.Exec (nil pool is safe).
func TestWriteBatchEmpty(t *testing.T) {
	h := &Historian{}
	if err := h.WriteBatch(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := h.WriteBatch(context.Background(), []core.Sample{}); err != nil {
		t.Fatal(err)
	}
}

func TestPing_NotConfiguredMessage(t *testing.T) {
	var h *Historian
	err := h.Ping(context.Background())
	if err == nil || err.Error() != "historian not configured" {
		t.Fatalf("nil historian: %v", err)
	}
	err = (&Historian{}).Ping(context.Background())
	if err == nil || err.Error() != "historian not configured" {
		t.Fatalf("nil pool: %v", err)
	}
}
