package store

import "testing"

func TestLive_ClearNil(t *testing.T) {
	if n := (*Live)(nil).Clear(); n != 0 {
		t.Fatalf("nil Clear=%d", n)
	}
}

func TestLive_MarkQualityEmpty(t *testing.T) {
	l := NewLive()
	if got := l.MarkQuality(nil, 0); got != nil {
		t.Fatalf("%#v", got)
	}
	if got := l.MarkQuality([]string{}, 0); got != nil {
		t.Fatalf("%#v", got)
	}
}
