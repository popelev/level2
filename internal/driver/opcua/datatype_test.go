package opcua

import (
	"testing"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

func TestMapOPCDataType_Boolean(t *testing.T) {
	nid := ua.NewNumericNodeID(0, 1)
	if got := mapOPCDataType(nid); got != core.ValueBool {
		t.Fatalf("got %q", got)
	}
}

func TestMapOPCDataType_DateTime(t *testing.T) {
	nid := ua.NewNumericNodeID(0, id.DateTime)
	if got := mapOPCDataType(nid); got != core.ValueDateTime {
		t.Fatalf("got %q", got)
	}
}

func TestMapOPCDataType_UInt(t *testing.T) {
	for _, tid := range []uint32{id.Byte, id.UInt16, id.UInt32, id.UInt64} {
		nid := ua.NewNumericNodeID(0, tid)
		if got := mapOPCDataType(nid); got != core.ValueUint {
			t.Fatalf("opc id %d → %q, want uint", tid, got)
		}
	}
	for _, tid := range []uint32{id.SByte, id.Int16, id.Int32, id.Int64} {
		nid := ua.NewNumericNodeID(0, tid)
		if got := mapOPCDataType(nid); got != core.ValueInt64 {
			t.Fatalf("opc id %d → %q, want int64", tid, got)
		}
	}
}

func TestGuessDataType_ModeFlags(t *testing.T) {
	if got := GuessDataType("group_mode_203_maintenance"); got != core.ValueBool {
		t.Fatalf("got %q", got)
	}
	if got := GuessDataType("anode_slime_time"); got != core.ValueDateTime {
		t.Fatalf("time suffix: got %q", got)
	}
}
