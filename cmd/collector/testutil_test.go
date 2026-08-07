package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/spool"
)

func testLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func testConfig(t testing.TB, devices ...core.Device) *config.Store {
	t.Helper()
	dir := t.TempDir()
	f := &config.File{
		Listen:   ":0",
		SpoolDir: dir,
		UIDir:    dir,
		Database: config.Database{
			URL:             "postgres://u:p@localhost/db",
			CapacityPercent: 90,
			FullPolicy:      config.FullPolicyStop,
		},
	}
	s := config.NewStore(filepath.Join(dir, "config.yaml"), f)
	for _, d := range devices {
		if err := s.UpsertDevice(d); err != nil {
			t.Fatalf("UpsertDevice: %v", err)
		}
	}
	return s
}

func testDevice(id string, tags ...core.Tag) core.Device {
	if id == "" {
		id = "plc"
	}
	return core.Device{
		ID:       id,
		Endpoint: "opc.tcp://127.0.0.1:4840",
		Security: "None",
		Tags:     tags,
	}
}

func testTag(id, nodeID string) core.Tag {
	if nodeID == "" {
		nodeID = "ns=4;i=1"
	}
	return core.Tag{
		ID:         id,
		NodeID:     nodeID,
		DataType:   core.ValueFloat64,
		Enabled:    true,
		IntervalMs: 1000,
	}
}

func testSpool(t testing.TB, maxN int) *spool.FileSpool {
	t.Helper()
	if maxN <= 0 {
		maxN = 50
	}
	sp, err := spool.New(t.TempDir(), maxN)
	if err != nil {
		t.Fatalf("spool: %v", err)
	}
	return sp
}

func sampleNum(tagID string, v float64, tm time.Time) core.Sample {
	if tm.IsZero() {
		tm = time.Now().UTC()
	}
	n := v
	return core.Sample{TagID: tagID, ValueNum: &n, Quality: core.QualityGood, Time: tm}
}

// memHist is an in-memory core.Historian for flush/replay tests.
type memHist struct {
	mu       sync.Mutex
	WriteErr error
	Batches  [][]core.Sample
}

func (h *memHist) EnsureSchema(context.Context) error { return nil }
func (h *memHist) Close(context.Context) error        { return nil }

func (h *memHist) WriteBatch(_ context.Context, samples []core.Sample) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.WriteErr != nil {
		return h.WriteErr
	}
	cp := append([]core.Sample(nil), samples...)
	h.Batches = append(h.Batches, cp)
	return nil
}

func (h *memHist) wrote() [][]core.Sample {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([][]core.Sample, len(h.Batches))
	copy(out, h.Batches)
	return out
}

// stubDriver is a non-OPC core.Driver for startDeviceCollect type-assert tests.
type stubDriver struct{ on bool }

func (d *stubDriver) Connect(context.Context) error    { return nil }
func (d *stubDriver) Disconnect(context.Context) error { return nil }
func (d *stubDriver) Connected() bool                  { return d.on }
func (d *stubDriver) Subscribe(ctx context.Context, _ []core.Tag, _ chan<- core.Sample) error {
	<-ctx.Done()
	return ctx.Err()
}
