package runtime

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
)

func testLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHub_SimUpsertStatusBrowser(t *testing.T) {
	h := NewHub(testLog(), true)
	ctx := context.Background()
	dev := core.Device{ID: "sim1", Endpoint: "opc.tcp://unused:4840"}

	if err := h.Upsert(ctx, dev); err != nil {
		t.Fatal(err)
	}
	st := h.Status()
	if !st["sim1"] {
		t.Fatalf("status=%v", st)
	}
	if !h.AnyConnected() {
		t.Fatal("AnyConnected")
	}
	br, err := h.Browser("sim1")
	if err != nil || br == nil {
		t.Fatalf("browser: %v %#v", err, br)
	}
	ent, ok := h.Entry("sim1")
	if !ok || ent.Driver == nil || !ent.Driver.Connected() {
		t.Fatalf("entry %#v ok=%v", ent, ok)
	}
	drivers := h.Drivers()
	if len(drivers) != 1 {
		t.Fatalf("drivers=%d", len(drivers))
	}
	if err := drivers[0].Subscribe(ctx, nil, nil); err == nil {
		t.Fatal("sim stub Subscribe should fail")
	}

	// Replace same id.
	if err := h.Upsert(ctx, core.Device{ID: "sim1", Endpoint: "opc.tcp://other:4840"}); err != nil {
		t.Fatal(err)
	}
	if len(h.Drivers()) != 1 {
		t.Fatalf("after replace drivers=%d", len(h.Drivers()))
	}

	h.Remove(ctx, "sim1")
	if _, ok := h.Entry("sim1"); ok {
		t.Fatal("expected removed")
	}
	if h.AnyConnected() {
		t.Fatal("no devices → not connected")
	}
	if _, err := h.Browser("sim1"); err == nil {
		t.Fatal("expected missing browser error")
	}
}

func TestHub_EmptyStatus(t *testing.T) {
	h := NewHub(testLog(), true)
	if len(h.Status()) != 0 {
		t.Fatalf("status=%v", h.Status())
	}
	if h.AnyConnected() {
		t.Fatal("empty hub")
	}
	// Remove unknown is no-op.
	h.Remove(context.Background(), "nope")
}

func TestAlwaysOnStub(t *testing.T) {
	a := &alwaysOn{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := a.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Disconnect(ctx); err != nil {
		t.Fatal(err)
	}
	if !a.Connected() {
		t.Fatal("connected")
	}
}
