package opcua

import (
	"testing"

	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

func TestPrepareTags_OK(t *testing.T) {
	tags := []core.Tag{
		{ID: "a", NodeID: "ns=4;i=4208", DataType: core.ValueFloat64},
		{ID: "b", NodeID: "ns=2;s=Foo.Bar", DataType: core.ValueString},
	}
	got, err := PrepareTags(tags)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Parsed.Namespace != 4 || got[0].Parsed.Identifier != "4208" {
		t.Fatalf("%#v", got[0].Parsed)
	}
	if got[1].Parsed.IdentifierType != "s" || got[1].Parsed.Identifier != "Foo.Bar" {
		t.Fatalf("%#v", got[1].Parsed)
	}
}

func TestPrepareTags_BadNodeOrType(t *testing.T) {
	if _, err := PrepareTags([]core.Tag{{ID: "x", NodeID: "not-a-node", DataType: core.ValueFloat64}}); err == nil {
		t.Fatal("expected parse error")
	}
	if _, err := PrepareTags([]core.Tag{{ID: "x", NodeID: "ns=4;i=1", DataType: "struct"}}); err == nil {
		t.Fatal("expected datatype error")
	}
}

func TestValidateLeafTag(t *testing.T) {
	ok := []core.ValueType{core.ValueBool, core.ValueInt64, core.ValueUint, core.ValueFloat64, core.ValueString, core.ValueDateTime}
	for _, dt := range ok {
		if err := ValidateLeafTag(TagView{Tag: core.Tag{ID: "t", DataType: dt}}); err != nil {
			t.Fatalf("%s: %v", dt, err)
		}
	}
	if err := ValidateLeafTag(TagView{Tag: core.Tag{ID: "t", DataType: "bytes"}}); err == nil {
		t.Fatal("expected error")
	}
}

func TestToUANodeID(t *testing.T) {
	n, err := toUANodeID(core.ParsedNodeID{Namespace: 4, IdentifierType: "i", Identifier: "4208"})
	if err != nil {
		t.Fatal(err)
	}
	if n.Namespace() != 4 || n.Type() != ua.NodeIDTypeNumeric || n.IntID() != 4208 {
		t.Fatalf("%#v", n)
	}
	s, err := toUANodeID(core.ParsedNodeID{Namespace: 2, IdentifierType: "s", Identifier: "Path.Leaf"})
	if err != nil {
		t.Fatal(err)
	}
	if s.StringID() != "Path.Leaf" {
		t.Fatalf("%q", s.StringID())
	}
	if _, err := toUANodeID(core.ParsedNodeID{IdentifierType: "g", Identifier: "x"}); err == nil {
		t.Fatal("guid unsupported in M1")
	}
}

func TestSanitizeTagID(t *testing.T) {
	got := sanitizeTagID("Tank Level/A-B.c")
	if got != "tank_level_a_b_c" {
		t.Fatalf("%q", got)
	}
}
