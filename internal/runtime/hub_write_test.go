package runtime

import (
	"context"
	"testing"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/driver/simbrowser"
)

type writeCapable struct {
	alwaysOn
	last any
}

func (w *writeCapable) WriteValue(_ context.Context, _ core.Tag, value any) error {
	w.last = value
	return nil
}

type readWriteCapable struct {
	writeCapable
	sample core.Sample
}

func (r *readWriteCapable) ReadValue(_ context.Context, tag core.Tag) (core.Sample, error) {
	s := r.sample
	if s.TagID == "" {
		s.TagID = tag.ID
	}
	return s, nil
}

func TestHub_InjectDriverAndValueWriter(t *testing.T) {
	h := NewHub(testLog(), true)
	ctx := context.Background()
	dev := core.Device{ID: "w1", Endpoint: "opc.tcp://unused:4840"}

	if _, err := h.ValueWriter("missing"); err == nil {
		t.Fatal("expected missing driver error")
	}
	if _, err := h.ValueReader("missing"); err == nil {
		t.Fatal("expected missing reader error")
	}

	drv := &readWriteCapable{sample: core.Sample{TagID: "t", Quality: core.QualityGood}}
	br := simbrowser.NewDemo()
	h.InjectDriver(dev, drv, br)

	w, err := h.ValueWriter("w1")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteValue(ctx, core.Tag{ID: "t"}, 42); err != nil {
		t.Fatal(err)
	}
	if drv.last != 42 {
		t.Fatalf("wrote %#v", drv.last)
	}
	rd, err := h.ValueReader("w1")
	if err != nil {
		t.Fatal(err)
	}
	s, err := rd.ReadValue(ctx, core.Tag{ID: "t"})
	if err != nil || s.TagID != "t" {
		t.Fatalf("read %#v err=%v", s, err)
	}

	// Non-writer driver.
	h.InjectDriver(dev, &alwaysOn{}, br)
	if _, err := h.ValueWriter("w1"); err == nil {
		t.Fatal("expected no write support")
	}
	if _, err := h.ValueReader("w1"); err == nil {
		t.Fatal("expected no read support")
	}

	// Nil driver → no driver error.
	h.InjectDriver(dev, nil, br)
	if _, err := h.ValueWriter("w1"); err == nil {
		t.Fatal("expected nil driver error")
	}

	// Replace disconnects previous (writeCapable already replaced; inject again).
	h.InjectDriver(dev, &alwaysOn{}, nil)
	if _, err := h.Browser("w1"); err == nil {
		t.Fatal("nil browser should error")
	}
	st := h.Status()
	if !st["w1"] {
		t.Fatalf("alwaysOn should report connected: %v", st)
	}
}

func TestHub_OPCNewEntryConnectFail(t *testing.T) {
	// useSim=false exercises opcua driver construction + failed connect path in newEntry.
	h := NewHub(testLog(), false)
	ctx := context.Background()
	dev := core.Device{ID: "opc1", Endpoint: "opc.tcp://127.0.0.1:1"}
	if err := h.Upsert(ctx, dev); err != nil {
		t.Fatal(err)
	}
	ent, ok := h.Entry("opc1")
	if !ok || ent.Driver == nil {
		t.Fatalf("entry %#v ok=%v", ent, ok)
	}
	if ent.Connected || h.Status()["opc1"] || h.AnyConnected() {
		t.Fatal("failed connect should leave Connected=false")
	}
	h.Remove(ctx, "opc1")
}
