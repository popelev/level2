package backoff

import (
	"testing"
	"time"
)

func TestNew_Defaults(t *testing.T) {
	b := New(0, 0)
	if b.Initial != time.Second {
		t.Fatalf("initial=%v", b.Initial)
	}
	if b.Max != time.Second {
		t.Fatalf("max=%v", b.Max)
	}
	if b.Factor != 2 {
		t.Fatalf("factor=%v", b.Factor)
	}
	// max < initial → clamp max to initial
	b2 := New(2*time.Second, time.Second)
	if b2.Max != 2*time.Second {
		t.Fatalf("clamped max=%v", b2.Max)
	}
}
