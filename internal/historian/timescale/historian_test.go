package timescale

import (
	"context"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
)

// Compile-time check and batch SQL builder smoke via empty batch.
func TestWriteBatchEmpty(t *testing.T) {
	h := &Historian{}
	if err := h.WriteBatch(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestSampleFields(t *testing.T) {
	f := 1.5
	s := core.Sample{Time: time.Now().UTC(), TagID: "x", ValueNum: &f, Quality: core.QualityGood}
	if s.ValueNum == nil {
		t.Fatal("expected num")
	}
}
