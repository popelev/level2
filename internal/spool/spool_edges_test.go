package spool

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestNew_DefaultMaxAndEmptyEnqueue(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if s.maxN != 1000 {
		t.Fatalf("maxN=%d", s.maxN)
	}
	if err := s.Enqueue(nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Enqueue([]core.Sample{}); err != nil {
		t.Fatal(err)
	}
	if s.Len() != 0 {
		t.Fatalf("len=%d", s.Len())
	}
}

func TestLoad_BadJSON(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(path); err == nil {
		t.Fatal("expected unmarshal error")
	}
}
