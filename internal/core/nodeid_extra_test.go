package core

import "testing"

func TestParseNodeID_GuidAndByteString(t *testing.T) {
	g, err := ParseNodeID("ns=2;g=09087e75-8e5e-499b-954f-f2a9603db28a")
	if err != nil {
		t.Fatal(err)
	}
	if g.IdentifierType != "g" || g.Namespace != 2 {
		t.Fatalf("%#v", g)
	}
	b, err := ParseNodeID("ns=1;b=SGVsbG8=")
	if err != nil {
		t.Fatal(err)
	}
	if b.IdentifierType != "b" || b.Identifier != "SGVsbG8=" {
		t.Fatalf("%#v", b)
	}
	s0, err := ParseNodeID("s=Objects")
	if err != nil {
		t.Fatal(err)
	}
	if s0.Namespace != 0 || s0.IdentifierType != "s" {
		t.Fatalf("%#v", s0)
	}
}

func TestParseNodeID_MissingNamespace(t *testing.T) {
	if _, err := ParseNodeID("foo;i=1"); err == nil {
		t.Fatal("expected missing namespace")
	}
	if _, err := ParseNodeID("ns=1;i="); err == nil {
		t.Fatal("expected empty identifier")
	}
}

func TestNormalizeValueType_CanonicalDefault(t *testing.T) {
	// Hit default branch that re-checks already-normalized names after trim.
	if got := NormalizeValueType("  float64  "); got != ValueFloat64 {
		t.Fatalf("%q", got)
	}
	if got := NormalizeValueType("unsigned"); got != ValueUint {
		t.Fatalf("%q", got)
	}
	if got := NormalizeValueType("unsignedint"); got != ValueUint {
		t.Fatalf("%q", got)
	}
}
