package mock

import (
	"context"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
)

func TestNewDemo_DefaultInterval(t *testing.T) {
	d := NewDemo(0)
	if d.interval != time.Second {
		t.Fatalf("interval=%v want 1s", d.interval)
	}
	d2 := NewDemo(-5 * time.Millisecond)
	if d2.interval != time.Second {
		t.Fatalf("negative interval=%v want 1s", d2.interval)
	}
}

func TestSubscribe_CancelDuringSend(t *testing.T) {
	d := New()
	if err := d.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	n := 1.0
	d.SetValue("t1", core.Sample{ValueNum: &n, Quality: core.QualityGood})

	out := make(chan core.Sample) // unbuffered → send blocks
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Subscribe(ctx, []core.Tag{{ID: "t1", Enabled: true}}, out)
	}()

	time.Sleep(120 * time.Millisecond) // wait for ticker + blocked send
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected context error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscribe did not return after cancel")
	}
}
