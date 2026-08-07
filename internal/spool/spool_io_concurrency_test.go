package spool

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
)

func TestEnqueue_WriteFailsWhenDirRemoved(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	err = s.Enqueue([]core.Sample{{TagID: "x", Time: time.Now().UTC()}})
	if err == nil {
		t.Fatal("expected write error after dir removed")
	}
}

func TestRemove_MissingFile(t *testing.T) {
	s, err := New(t.TempDir(), 10)
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "gone.json")
	if err := s.Remove(missing); err == nil {
		t.Fatal("expected remove error")
	}
}

func TestFileSpool_ConcurrentEnqueueList(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, 200)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Stagger so UnixNano filenames do not collide under load.
			time.Sleep(time.Duration(id) * time.Millisecond)
			n := float64(id)
			batch := []core.Sample{{
				TagID:    "t",
				ValueNum: &n,
				Time:     time.Now().UTC(),
				Quality:  core.QualityGood,
			}}
			if err := s.Enqueue(batch); err != nil {
				errCh <- err
				return
			}
			_, _ = s.List()
			_ = s.Len()
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
	if s.Len() != 8 {
		t.Fatalf("len=%d", s.Len())
	}
}
