package spool

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
)

func TestFileSpool_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	n := 42.0
	batch := []core.Sample{{
		Time:     time.Unix(1, 0).UTC(),
		TagID:    "t1",
		ValueNum: &n,
		Quality:  core.QualityGood,
	}}
	if err := s.Enqueue(batch); err != nil {
		t.Fatal(err)
	}
	if s.Len() != 1 {
		t.Fatalf("len=%d", s.Len())
	}
	files, err := s.List()
	if err != nil || len(files) != 1 {
		t.Fatalf("list: %v %v", files, err)
	}
	got, err := s.Load(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TagID != "t1" || got[0].ValueNum == nil || *got[0].ValueNum != 42 {
		t.Fatalf("unexpected %#v", got)
	}
	if err := s.Remove(files[0]); err != nil {
		t.Fatal(err)
	}
	if s.Len() != 0 {
		t.Fatal("expected empty after remove")
	}
	_ = filepath.Base(files[0])
}

func TestFileSpool_Full(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Enqueue([]core.Sample{{TagID: "a", Time: time.Now().UTC()}})
	if err := s.Enqueue([]core.Sample{{TagID: "b", Time: time.Now().UTC()}}); err == nil {
		t.Fatal("expected spool full")
	}
}
