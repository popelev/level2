package opcua

import (
	"testing"

	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

func TestFormatNodeID_TwoByteNS0(t *testing.T) {
	n := ua.NewTwoByteNodeID(0, 85)
	got := formatNodeID(n)
	if got != "ns=0;i=85" {
		t.Fatalf("got %q want ns=0;i=85", got)
	}
}

func TestExpandFromTree_LeavesOnly(t *testing.T) {
	nodes := []core.BrowseNode{
		{NodeID: "ns=4;i=4208", BrowseName: "rValueOut", IsLeaf: true},
		{NodeID: "ns=4;i=4209", BrowseName: "sUnit", IsLeaf: true},
		{NodeID: "ns=4;i=4210", BrowseName: "Nested", IsLeaf: false},
	}
	got := ExpandFromTree("tank", nodes)
	if len(got) != 2 {
		t.Fatalf("got %d", len(got))
	}
	if got[0].ID != "tank_rvalueout" || got[0].DataType != core.ValueFloat64 {
		t.Fatalf("unexpected %#v", got[0])
	}
	if got[1].ID != "tank_sunit" || got[1].DataType != core.ValueString {
		t.Fatalf("unexpected %#v", got[1])
	}
}

func TestGuessDataType(t *testing.T) {
	cases := map[string]core.ValueType{
		"rValueOut": core.ValueFloat64,
		"sUnit":     core.ValueString,
		"bEnable":   core.ValueBool,
		"iCount":    core.ValueInt64,
	}
	for name, want := range cases {
		if got := guessDataType(name); got != want {
			t.Fatalf("%s: got %s want %s", name, got, want)
		}
	}
}
