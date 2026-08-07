package runtime

import (
	"context"
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestHub_StatusReflectsReconnect(t *testing.T) {
	h := NewHub(testLog(), true)
	dev := core.Device{ID: "r1", Endpoint: "opc.tcp://x"}
	d := &flippingDriver{on: false}
	h.InjectDriver(dev, d, nil)

	st := h.Status()
	if st["r1"] {
		t.Fatal("expected offline")
	}
	ent, ok := h.Entry("r1")
	if !ok || ent.Connected {
		t.Fatalf("entry connected should follow status sync: %#v", ent)
	}

	d.on = true
	st = h.Status()
	if !st["r1"] {
		t.Fatal("expected online after flip")
	}
	ent, _ = h.Entry("r1")
	if !ent.Connected {
		t.Fatal("Status should sync Entry.Connected")
	}
	if !h.AnyConnected() {
		t.Fatal("AnyConnected")
	}
}

type flippingDriver struct {
	on bool
}

func (f *flippingDriver) Connect(context.Context) error    { return nil }
func (f *flippingDriver) Disconnect(context.Context) error { return nil }
func (f *flippingDriver) Connected() bool                  { return f.on }
func (f *flippingDriver) Subscribe(context.Context, []core.Tag, chan<- core.Sample) error {
	return nil
}
