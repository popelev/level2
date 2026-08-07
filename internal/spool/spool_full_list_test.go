package spool

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
)

func TestEnqueue_FullAndListOrder(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	n := 1.0
	if err := s.Enqueue([]core.Sample{{TagID: "a", ValueNum: &n, Quality: core.QualityGood, Time: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := s.Enqueue([]core.Sample{{TagID: "b", ValueNum: &n, Quality: core.QualityGood, Time: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Enqueue([]core.Sample{{TagID: "c", ValueNum: &n}}); err == nil {
		t.Fatal("expected spool full")
	}
	files, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || s.Len() != 2 {
		t.Fatalf("files=%d len=%d", len(files), s.Len())
	}
	loaded, err := s.Load(files[0])
	if err != nil || len(loaded) != 1 || loaded[0].TagID != "a" {
		t.Fatalf("load %#v err=%v", loaded, err)
	}
	if err := s.Remove(files[0]); err != nil {
		t.Fatal(err)
	}
	if s.Len() != 1 {
		t.Fatalf("after remove len=%d", s.Len())
	}
	// List still works after removing one file via os (not Remove).
	remain, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(remain) != 1 {
		t.Fatalf("remain=%v", remain)
	}
	_ = os.Remove(remain[0])
	if s.Len() != 0 {
		t.Fatalf("empty len=%d", s.Len())
	}
}
