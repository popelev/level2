package core

import "testing"

func TestParseNodeID_Numeric(t *testing.T) {
	p, err := ParseNodeID("ns=4;i=4208")
	if err != nil {
		t.Fatal(err)
	}
	if p.Namespace != 4 || p.IdentifierType != "i" || p.Identifier != "4208" {
		t.Fatalf("unexpected %#v", p)
	}
}

func TestParseNodeID_String(t *testing.T) {
	p, err := ParseNodeID(`ns=3;s="DB".Temp`)
	if err != nil {
		t.Fatal(err)
	}
	if p.Namespace != 3 || p.IdentifierType != "s" || p.Identifier != `"DB".Temp` {
		t.Fatalf("unexpected %#v", p)
	}
}

func TestParseNodeID_Invalid(t *testing.T) {
	cases := []string{"", "i=1", "ns=4", "ns=x;i=1", "ns=4;i=abc"}
	for _, c := range cases {
		if _, err := ParseNodeID(c); err == nil {
			t.Fatalf("expected error for %q", c)
		}
	}
}

func TestTruncateString(t *testing.T) {
	s, trunc := TruncateString("hi")
	if trunc || s != "hi" {
		t.Fatal("short string should not truncate")
	}
	long := make([]byte, MaxStringBytes+10)
	for i := range long {
		long[i] = 'a'
	}
	out, trunc := TruncateString(string(long))
	if !trunc || len(out) != MaxStringBytes {
		t.Fatalf("got len=%d trunc=%v", len(out), trunc)
	}
}
