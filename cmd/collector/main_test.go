package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	"github.com/popelev/level2/internal/historian/timescale"
	"github.com/popelev/level2/internal/spool"
)

func TestSelectSimTags(t *testing.T) {
	all := []core.Tag{
		{ID: "a", Enabled: true, Simulate: false},
		{ID: "b", Enabled: true, Simulate: true},
		{ID: "c", Enabled: false, Simulate: true},
		{ID: "d", Enabled: true, Simulate: true},
	}
	got := selectSimTags(all, false)
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "d" {
		t.Fatalf("%#v", got)
	}
	gotAll := selectSimTags(all, true)
	if len(gotAll) != 3 {
		t.Fatalf("allEnabled=%#v", gotAll)
	}
}

func TestSelectOPCTags(t *testing.T) {
	all := []core.Tag{
		{ID: "opc", Simulate: false},
		{ID: "sim", Simulate: true},
		{ID: "opc2", Simulate: false},
	}
	got := selectOPCTags(all)
	if len(got) != 2 || got[0].ID != "opc" || got[1].ID != "opc2" {
		t.Fatalf("%#v", got)
	}
}

func TestWatchConfig_CancelsOnGenChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	s := config.NewStore(path, &config.File{
		Listen: ":0",
		Devices: []core.Device{{
			ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None",
			Tags: []core.Tag{{ID: "t", NodeID: "ns=1;i=1", DataType: core.ValueFloat64, Enabled: true}},
		}},
	})
	gen := s.Gen()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		watchConfig(ctx, cancel, s, gen)
		close(done)
	}()
	if err := s.UpsertTag("plc", core.Tag{
		ID: "t2", NodeID: "ns=1;i=2", DataType: core.ValueBool, Enabled: true, IntervalMs: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("watchConfig did not cancel after gen bump")
	}
}

func TestWatchConfig_StopsOnParentCancel(t *testing.T) {
	dir := t.TempDir()
	s := config.NewStore(filepath.Join(dir, "c.yaml"), &config.File{Listen: ":0"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		watchConfig(ctx, func() {}, s, s.Gen())
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchConfig did not exit on ctx cancel")
	}
}

func TestWatchReady_NilAndTransition(t *testing.T) {
	watchReady(context.Background(), nil) // no-op

	inc := diag.NewIncidentTracker(64, time.Hour)
	diag.SetDefaultIncidents(inc)

	var ready atomic.Bool
	ready.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watchReady(ctx, func() bool { return ready.Load() })

	// Allow first sample of was=true
	time.Sleep(50 * time.Millisecond)
	ready.Store(false)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if inc.Count(diag.IncidentCollectorDown, time.Hour) > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("expected collector_not_ready incident")
}

type stubHist struct {
	err   error
	wrote [][]core.Sample
}

func (s *stubHist) EnsureSchema(context.Context) error { return nil }
func (s *stubHist) Close(context.Context) error        { return nil }
func (s *stubHist) WriteBatch(_ context.Context, samples []core.Sample) error {
	if s.err != nil {
		return s.err
	}
	cp := append([]core.Sample(nil), samples...)
	s.wrote = append(s.wrote, cp)
	return nil
}

func TestFlushLoop_WriteAndSpoolOnError(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sp, err := spool.New(t.TempDir(), 50)
	if err != nil {
		t.Fatal(err)
	}
	hist := &stubHist{}
	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan core.Sample, 8)
	done := make(chan struct{})
	go func() {
		flushLoop(ctx, log, hist, sp, in)
		close(done)
	}()
	v1 := 1.0
	in <- core.Sample{TagID: "a", ValueNum: &v1, Quality: core.QualityGood, Time: time.Now().UTC()}
	// Wait for ticker flush
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(hist.wrote) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(hist.wrote) == 0 {
		t.Fatal("expected successful write")
	}

	hist.err = errors.New("db down")
	v2 := 2.0
	in <- core.Sample{TagID: "b", ValueNum: &v2, Quality: core.QualityGood, Time: time.Now().UTC()}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if sp.Len() > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if sp.Len() == 0 {
		t.Fatal("expected spool enqueue after write error")
	}

	hist.err = timescale.ErrCapacityHalt
	before := sp.Len()
	v3 := 3.0
	in <- core.Sample{TagID: "c", ValueNum: &v3, Quality: core.QualityGood, Time: time.Now().UTC()}
	time.Sleep(1200 * time.Millisecond)
	if sp.Len() != before {
		t.Fatalf("capacity halt must not spool: before=%d after=%d", before, sp.Len())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("flushLoop did not exit")
	}
}

func TestReplaySpool_CorruptFileRemoved(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	dir := t.TempDir()
	sp, err := spool.New(dir, 50)
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "0001.json")
	if err := os.WriteFile(bad, []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	hist := &stubHist{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		replaySpool(ctx, log, hist, sp)
		close(done)
	}()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(bad); os.IsNotExist(err) {
			cancel()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("replaySpool did not exit")
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	t.Fatal("corrupt spool file was not removed")
}

func TestReplaySpool_SuccessAndHalt(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	dir := t.TempDir()
	sp, err := spool.New(dir, 50)
	if err != nil {
		t.Fatal(err)
	}
	v := 9.0
	batch := []core.Sample{{TagID: "x", ValueNum: &v, Quality: core.QualityGood, Time: time.Now().UTC()}}
	if err := sp.Enqueue(batch); err != nil {
		t.Fatal(err)
	}
	hist := &stubHist{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		replaySpool(ctx, log, hist, sp)
		close(done)
	}()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if sp.Len() == 0 && len(hist.wrote) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if sp.Len() != 0 || len(hist.wrote) == 0 {
		t.Fatalf("replay failed: spool=%d wrote=%d", sp.Len(), len(hist.wrote))
	}

	if err := sp.Enqueue(batch); err != nil {
		t.Fatal(err)
	}
	hist.err = timescale.ErrCapacityHalt
	time.Sleep(5500 * time.Millisecond)
	if sp.Len() == 0 {
		t.Fatal("capacity halt must leave spool file")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("replaySpool did not exit")
	}
}
