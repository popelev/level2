package backoff

import (
	"testing"
	"time"
)

func TestExp_GrowsAndCaps(t *testing.T) {
	b := New(time.Second, 4*time.Second)
	got := []time.Duration{b.Next(), b.Next(), b.Next(), b.Next()}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("step %d: got %v want %v", i, got[i], want[i])
		}
	}
	b.Reset()
	if b.Next() != time.Second {
		t.Fatal("reset should restart at initial")
	}
}
