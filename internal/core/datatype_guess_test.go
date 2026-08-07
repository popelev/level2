package core

import "testing"

func TestLooksLikeStringNameTable(t *testing.T) {
	// looksLikeStringName is called with lowercased hints from GuessDataType.
	yes := []string{"sunit", "tank_sunit", "unit", "name", "text", "sunitx", "foo.sname", "a_stext"}
	for _, n := range yes {
		if !looksLikeStringName(n) {
			t.Fatalf("want true: %q", n)
		}
	}
	no := []string{"rvalueout", "count", "benable", "runtime"}
	for _, n := range no {
		if looksLikeStringName(n) {
			t.Fatalf("want false: %q", n)
		}
	}
}

func TestGuessDataType_Smoke(t *testing.T) {
	if got := GuessDataType("sUnit"); got != ValueString {
		t.Fatalf("sUnit: %q", got)
	}
	if got := GuessDataType("bEnable"); got != ValueBool {
		t.Fatalf("bEnable: %q", got)
	}
	if got := GuessDataType("rValueOut"); got != ValueFloat64 {
		t.Fatalf("rValueOut: %q", got)
	}
}
