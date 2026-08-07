package spool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNew_MkdirFail(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(filepath.Join(blocker, "child"), 10); err == nil {
		t.Fatal("expected MkdirAll error")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	s, err := New(t.TempDir(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected read error")
	}
}
