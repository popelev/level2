package core

import "testing"

func TestNormalizeValueType_DateTime(t *testing.T) {
	if got := NormalizeValueType("date_time"); got != ValueDateTime {
		t.Fatalf("got %q", got)
	}
}
