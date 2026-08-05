package mock

import (
	"context"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
)

func TestDemoSubscribe_EmitsFloat(t *testing.T) {
	d := NewDemo(20 * time.Millisecond)
	if err := d.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	out := make(chan core.Sample, 8)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go func() {
		_ = d.Subscribe(ctx, []core.Tag{{
			ID: "opc_measure_rvalue", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 20,
		}}, out)
	}()
	select {
	case s := <-out:
		if s.Quality != core.QualityGood || s.ValueNum == nil {
			t.Fatalf("unexpected %#v", s)
		}
		if *s.ValueNum < 80 || *s.ValueNum > 100 {
			t.Fatalf("value out of demo range: %v", *s.ValueNum)
		}
	case <-ctx.Done():
		t.Fatal("no sample")
	}
}
