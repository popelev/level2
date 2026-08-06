package opcua

import (
	"context"
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

func TestMapOPCDataType_StringTypes(t *testing.T) {
	for _, tid := range []uint32{id.String, id.LocalizedText, id.ByteString, id.XMLElement} {
		nid := ua.NewNumericNodeID(0, tid)
		if got := mapOPCDataType(nid); got != core.ValueString {
			t.Fatalf("opc id %d → %q, want string", tid, got)
		}
	}
	if id.String != 12 {
		t.Fatalf("String node id: got %d want 12", id.String)
	}
}

func TestMapOPCDataType_DateTime(t *testing.T) {
	nid := ua.NewNumericNodeID(0, id.DateTime)
	if got := mapOPCDataType(nid); got != core.ValueDateTime {
		t.Fatalf("got %q", got)
	}
	utc := ua.NewNumericNodeID(0, id.UtcTime) // i=294 subtype of DateTime
	if got := mapOPCDataType(utc); got != core.ValueDateTime {
		t.Fatalf("UtcTime: got %q", got)
	}
	if id.DateTime != 13 {
		t.Fatalf("DateTime node id: got %d want 13", id.DateTime)
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

func TestGuessDataType_SUnitFullTagID(t *testing.T) {
	cases := []string{
		"sUnit",
		"unit",
		"tank_sunit",
		"objects_serverinterfaces_tankhouse_data_1_e1_ece_201_current_sunit",
		"OilTemp.sUnit",
		"sName",
		"sText",
	}
	for _, name := range cases {
		if got := GuessDataType(name); got != core.ValueString {
			t.Fatalf("%q: got %q want string", name, got)
		}
	}
	// Analog / non-string must not flip.
	if got := GuessDataType("rValueOut"); got != core.ValueFloat64 {
		t.Fatalf("rValueOut: got %q", got)
	}
	if got := GuessDataType("objects_serverinterfaces_tankhouse_data_1_e1_ece_201_current_rvalueout"); got != core.ValueFloat64 {
		t.Fatalf("full rvalueout id: got %q", got)
	}
}

func TestGuessDataType_SiemensDateAndTime(t *testing.T) {
	cases := []string{
		"LastCycleDateAndTime",
		"lastcycledateandtime",
		"objects_serverinterfaces_tankhouse_data_2_e3_machines_metso_machines_csm_lastcycledateandtime",
		"DATE_AND_TIME",
		"MyDate_Time",
		"CycleDateTime",
	}
	for _, name := range cases {
		if got := GuessDataType(name); got != core.ValueDateTime {
			t.Fatalf("%q: got %q want datetime", name, got)
		}
	}
}

func TestResolveMappedDataType_ByteStringSiemensDT(t *testing.T) {
	// OPC Attribute DataType = ByteString → string; name says DATE_AND_TIME → datetime.
	got := resolveMappedDataType(core.ValueString, "LastCycleDateAndTime")
	if got != core.ValueDateTime {
		t.Fatalf("got %q", got)
	}
	// Real string node stays string.
	got = resolveMappedDataType(core.ValueString, "sUnit")
	if got != core.ValueString {
		t.Fatalf("sUnit: got %q", got)
	}
	// Authoritative OPC Float is kept even if name looks like time.
	got = resolveMappedDataType(core.ValueFloat64, "LastCycleDateAndTime")
	if got != core.ValueFloat64 {
		t.Fatalf("float kept: got %q", got)
	}
	// Vendor ns / unmapped → Guess.
	got = resolveMappedDataType("", "LastCycleDateAndTime")
	if got != core.ValueDateTime {
		t.Fatalf("empty map: got %q", got)
	}
	got = resolveMappedDataType(core.ValueDateTime, "anything")
	if got != core.ValueDateTime {
		t.Fatalf("opc DateTime: got %q", got)
	}
}

func TestApplyDataTypesFromOPC_OverwritesWrongFloat64(t *testing.T) {
	// Disconnected driver → empty OPC map → Guess; Sync must overwrite existing float64.
	d := &Driver{}
	tags := []core.Tag{{
		ID:       "objects_serverinterfaces_tankhouse_data_2_e3_machines_metso_machines_csm_lastcycledateandtime",
		NodeID:   "ns=4;i=6156",
		DataType: core.ValueFloat64,
	}}
	ApplyDataTypesFromOPC(context.Background(), d, tags)
	if tags[0].DataType != core.ValueDateTime {
		t.Fatalf("sync overwrite: got %q want datetime", tags[0].DataType)
	}
}

func TestApplyDataTypesFromOPC_OverwritesWrongFloat64_SUnit(t *testing.T) {
	d := &Driver{}
	tags := []core.Tag{{
		ID:       "objects_serverinterfaces_tankhouse_data_1_e1_ece_201_current_sunit",
		NodeID:   "ns=5;i=7460",
		DataType: core.ValueFloat64,
	}}
	ApplyDataTypesFromOPC(context.Background(), d, tags)
	if tags[0].DataType != core.ValueString {
		t.Fatalf("sync overwrite sUnit: got %q want string", tags[0].DataType)
	}
}

func TestResolveMappedDataType_SUnitAmbiguous(t *testing.T) {
	got := resolveMappedDataType("", "objects_serverinterfaces_tankhouse_data_1_e1_ece_201_current_sunit")
	if got != core.ValueString {
		t.Fatalf("empty map full id: got %q", got)
	}
	got = resolveMappedDataType(core.ValueString, "sUnit")
	if got != core.ValueString {
		t.Fatalf("opc String: got %q", got)
	}
	// Authoritative OPC Float kept (ambiguous-name refine only when unmapped).
	got = resolveMappedDataType(core.ValueFloat64, "sUnit")
	if got != core.ValueFloat64 {
		t.Fatalf("float kept: got %q", got)
	}
}

func TestBrowseNameHint(t *testing.T) {
	got := browseNameHint(core.ExpandedTag{BrowsePath: "TankHouse.Data.Dcswitch", ID: "x_dcswitch"})
	if got != "Dcswitch" {
		t.Fatalf("got %q", got)
	}
	got = browseNameHint(core.ExpandedTag{ID: "tank_dcswitch"})
	if got != "tank_dcswitch" {
		t.Fatalf("id fallback: got %q", got)
	}
}

func TestFillExpandedDataTypes_GuessFallbackWhenDisconnected(t *testing.T) {
	d := &Driver{} // no client — batch read yields empty → Guess fallback
	tags := []core.ExpandedTag{
		{ID: "a_sunit", NodeID: "ns=4;i=1", BrowsePath: "sUnit"},
		{ID: "a_benable", NodeID: "ns=4;i=2", BrowsePath: "bEnable"},
		{ID: "a_rvalue", NodeID: "ns=4;i=3", BrowsePath: "rValueOut"},
		{ID: "a_dt", NodeID: "ns=4;i=6156", BrowsePath: "LastCycleDateAndTime"},
	}
	d.fillExpandedDataTypes(context.Background(), tags, nil)
	if tags[0].DataType != core.ValueString {
		t.Fatalf("sUnit: got %q", tags[0].DataType)
	}
	if tags[1].DataType != core.ValueBool {
		t.Fatalf("bEnable: got %q", tags[1].DataType)
	}
	if tags[2].DataType != core.ValueFloat64 {
		t.Fatalf("rValueOut: got %q", tags[2].DataType)
	}
	if tags[3].DataType != core.ValueDateTime {
		t.Fatalf("LastCycleDateAndTime: got %q", tags[3].DataType)
	}
}

