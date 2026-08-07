package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	"github.com/popelev/level2/internal/historian/timescale"
	"github.com/popelev/level2/internal/spool"
)

func TestProcessReady(t *testing.T) {
	cases := []struct {
		sim, conn, want bool
	}{
		{false, false, false},
		{true, false, true},
		{false, true, true},
		{true, true, true},
	}
	for _, tc := range cases {
		if got := processReady(tc.sim, tc.conn); got != tc.want {
			t.Fatalf("sim=%v conn=%v got %v want %v", tc.sim, tc.conn, got, tc.want)
		}
	}
}

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
	s := testConfig(t, testDevice("plc", testTag("t", "ns=1;i=1")))
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
	s := testConfig(t)
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
	inc := diag.NewIncidentTracker(64, time.Hour)
	diag.SetDefaultIncidents(inc)
	t.Cleanup(func() { diag.SetDefaultIncidents(diag.NewIncidentTracker(100, 0)) })

	watchReady(context.Background(), nil)
	if inc.Count(diag.IncidentCollectorDown, time.Hour) != 0 {
		t.Fatal("nil ready must not record incidents")
	}

	var ready atomic.Bool
	ready.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watchReady(ctx, func() bool { return ready.Load() })

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

func TestFlushLoop_WriteAndSpoolOnError(t *testing.T) {
	log := testLog()
	sp := testSpool(t, 50)
	hist := &memHist{}
	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan core.Sample, 8)
	done := make(chan struct{})
	go func() {
		flushLoop(ctx, log, hist, sp, in)
		close(done)
	}()
	in <- sampleNum("a", 1, time.Time{})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(hist.wrote()) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(hist.wrote()) == 0 {
		t.Fatal("expected successful write")
	}

	hist.WriteErr = errors.New("db down")
	in <- sampleNum("b", 2, time.Time{})
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

	hist.WriteErr = timescale.ErrCapacityHalt
	before := sp.Len()
	in <- sampleNum("c", 3, time.Time{})
	time.Sleep(1200 * time.Millisecond)
	if sp.Len() != before {
		t.Fatalf("capacity halt must not spool: before=%d after=%d", before, sp.Len())
	}

	hist.WriteErr = timescale.ErrCapacityBusy
	before = sp.Len()
	in <- sampleNum("d", 4, time.Time{})
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if sp.Len() > before {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if sp.Len() <= before {
		t.Fatalf("capacity busy must spool: before=%d after=%d", before, sp.Len())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("flushLoop did not exit")
	}
}

func TestReplaySpool_CorruptFileRemoved(t *testing.T) {
	log := testLog()
	dir := t.TempDir()
	sp, err := spool.New(dir, 50)
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "0001.json")
	if err := os.WriteFile(bad, []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	hist := &memHist{}
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
	log := testLog()
	sp := testSpool(t, 50)
	batch := []core.Sample{sampleNum("x", 9, time.Time{})}
	if err := sp.Enqueue(batch); err != nil {
		t.Fatal(err)
	}
	hist := &memHist{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		replaySpool(ctx, log, hist, sp)
		close(done)
	}()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if sp.Len() == 0 && len(hist.wrote()) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if sp.Len() != 0 || len(hist.wrote()) == 0 {
		t.Fatalf("replay failed: spool=%d wrote=%d", sp.Len(), len(hist.wrote()))
	}

	if err := sp.Enqueue(batch); err != nil {
		t.Fatal(err)
	}
	hist.WriteErr = timescale.ErrCapacityHalt
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
