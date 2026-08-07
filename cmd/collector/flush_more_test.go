package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/historian/timescale"
	"github.com/popelev/level2/internal/spool"
)

func TestReplaySpool_WriteErrorAndBusy(t *testing.T) {
	log := testLog()
	sp := testSpool(t, 50)
	batch := []core.Sample{sampleNum("y", 1, time.Time{})}
	if err := sp.Enqueue(batch); err != nil {
		t.Fatal(err)
	}
	hist := &memHist{WriteErr: errors.New("db flake")}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		replaySpool(ctx, log, hist, sp)
		close(done)
	}()
	time.Sleep(5500 * time.Millisecond)
	if sp.Len() == 0 {
		t.Fatal("generic write error must leave spool file")
	}

	hist.WriteErr = timescale.ErrCapacityBusy
	time.Sleep(5500 * time.Millisecond)
	if sp.Len() == 0 {
		t.Fatal("capacity busy must leave spool file")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("replaySpool did not exit")
	}
}

func TestFlushLoop_SpoolEnqueueFull(t *testing.T) {
	log := testLog()
	dir := t.TempDir()
	sp, err := spool.New(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := sp.Enqueue([]core.Sample{sampleNum("fill", 1, time.Time{})}); err != nil {
		t.Fatal(err)
	}
	hist := &memHist{WriteErr: errors.New("down")}
	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan core.Sample, 4)
	done := make(chan struct{})
	go func() {
		flushLoop(ctx, log, hist, sp, in)
		close(done)
	}()
	in <- sampleNum("z", 2, time.Time{})
	time.Sleep(1200 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("flushLoop did not exit")
	}
}

func TestReplaySpool_EmptyList(t *testing.T) {
	log := testLog()
	sp := testSpool(t, 10)
	hist := &memHist{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		replaySpool(ctx, log, hist, sp)
		close(done)
	}()
	time.Sleep(5200 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("replaySpool empty did not exit")
	}
}

func TestReplaySpool_PicksOldestFile(t *testing.T) {
	log := testLog()
	dir := t.TempDir()
	sp, err := spool.New(dir, 50)
	if err != nil {
		t.Fatal(err)
	}
	older := filepath.Join(dir, "0001.json")
	newer := filepath.Join(dir, "0002.json")
	if err := os.WriteFile(older, []byte(`[{"tag_id":"old","quality":0}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte(`[{"tag_id":"new","quality":0}]`), 0o644); err != nil {
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
		if _, err := os.Stat(older); os.IsNotExist(err) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(older); !os.IsNotExist(err) {
		cancel()
		t.Fatal("expected oldest spool file to be replayed first")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("replaySpool did not exit")
	}
}
