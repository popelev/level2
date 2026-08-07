package opcua

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

func TestMapOPCDataType_FloatAndEdges(t *testing.T) {
	if mapOPCDataType(nil) != "" {
		t.Fatal("nil")
	}
	vendor := ua.NewNumericNodeID(3, 1000)
	if mapOPCDataType(vendor) != "" {
		t.Fatal("vendor ns should be empty")
	}
	for _, tid := range []uint32{id.Float, id.Double} {
		if got := mapOPCDataType(ua.NewNumericNodeID(0, tid)); got != core.ValueFloat64 {
			t.Fatalf("id %d → %q", tid, got)
		}
	}
	if got := mapOPCDataType(ua.NewNumericNodeID(0, 99999)); got != "" {
		t.Fatalf("unknown → %q", got)
	}
}

func TestApplyDataTypesFromOPC_NilAndEmpty(t *testing.T) {
	ApplyDataTypesFromOPC(context.Background(), nil, []core.Tag{{ID: "x"}})
	ApplyDataTypesFromOPC(context.Background(), &Driver{}, nil)
	ApplyDataTypesFromOPC(context.Background(), &Driver{}, []core.Tag{})
}

func TestReadOPCDataTypesBatch_NoClient(t *testing.T) {
	d := &Driver{}
	var calls [][2]int
	out := d.readOPCDataTypesBatch(context.Background(), []string{"ns=4;i=1", "bad"}, func(done, total int) {
		calls = append(calls, [2]int{done, total})
	})
	if len(out) != 2 || out[0] != "" || out[1] != "" {
		t.Fatalf("%#v", out)
	}
	if len(calls) != 1 || calls[0] != [2]int{2, 2} {
		t.Fatalf("onChunk=%v", calls)
	}
	empty := d.readOPCDataTypesBatch(context.Background(), nil, nil)
	if len(empty) != 0 {
		t.Fatalf("%#v", empty)
	}
}

func TestResolveTagDataType_DisconnectedGuess(t *testing.T) {
	d := &Driver{}
	got := d.ResolveTagDataType(context.Background(), "ns=5;i=1", "sUnit")
	if got != core.ValueString {
		t.Fatalf("got %q", got)
	}
}

func TestAsNumericHelpers(t *testing.T) {
	if v, err := asFloat64(float64(1.25)); err != nil || v != 1.25 {
		t.Fatalf("f64 %v %v", v, err)
	}
	if v, err := asFloat64(int32(3)); err != nil || v != 3 {
		t.Fatalf("i32 %v %v", v, err)
	}
	if _, err := asFloat64(uint64(math.MaxUint64)); err == nil {
		t.Fatal("uint64 overflow")
	}
	if _, err := asFloat64("x"); err == nil {
		t.Fatal("string float")
	}

	if v, err := asInt64(int16(-2)); err != nil || v != -2 {
		t.Fatalf("i16 %v %v", v, err)
	}
	if v, err := asInt64(uint32(9)); err != nil || v != 9 {
		t.Fatalf("u32 %v %v", v, err)
	}
	if _, err := asInt64(true); err == nil {
		t.Fatal("bool int")
	}

	if v, err := asUint64(uint16(7)); err != nil || v != 7 {
		t.Fatalf("u16 %v %v", v, err)
	}
	if _, err := asUint64(int32(-1)); err == nil {
		t.Fatal("neg uint")
	}
	if _, err := asUint64("x"); err == nil {
		t.Fatal("string uint")
	}
}

func TestAsString_XMLElementAndErrors(t *testing.T) {
	s, err := asString(ua.XMLElement("<a/>"))
	if err != nil || s != "<a/>" {
		t.Fatalf("%q %v", s, err)
	}
	s, err = asString(ua.ByteArray{'o', 'k'})
	if err != nil || s != "ok" {
		t.Fatalf("%q %v", s, err)
	}
	var nilLT *ua.LocalizedText
	if _, err := asString(nilLT); err == nil {
		t.Fatal("nil LT")
	}
	if _, err := asString(123); err == nil {
		t.Fatal("int string")
	}
}

func TestMapBoolIntAndBadStatus(t *testing.T) {
	now := time.Now().UTC()

	bv, err := ua.NewVariant(true)
	if err != nil {
		t.Fatal(err)
	}
	s, err := mapDataValue(TagView{Tag: core.Tag{ID: "b", DataType: core.ValueBool}},
		&ua.DataValue{Status: ua.StatusOK, Value: bv}, now)
	if err != nil || s.ValueBool == nil || !*s.ValueBool {
		t.Fatalf("%#v %v", s, err)
	}

	iv, err := ua.NewVariant(int32(-5))
	if err != nil {
		t.Fatal(err)
	}
	s, err = mapDataValue(TagView{Tag: core.Tag{ID: "i", DataType: core.ValueInt64}},
		&ua.DataValue{Status: ua.StatusOK, Value: iv, SourceTimestamp: time.Unix(50, 0).UTC()}, now)
	if err != nil || s.ValueNum == nil || *s.ValueNum != -5 {
		t.Fatalf("%#v %v", s, err)
	}
	if !s.Time.Equal(time.Unix(50, 0).UTC()) {
		t.Fatalf("source ts %v", s.Time)
	}

	_, err = mapDataValue(TagView{Tag: core.Tag{ID: "x", DataType: core.ValueFloat64}}, nil, now)
	if err == nil {
		t.Fatal("nil dv")
	}
	_, err = mapDataValue(TagView{Tag: core.Tag{ID: "x", DataType: core.ValueFloat64}},
		&ua.DataValue{Status: ua.StatusBad}, now)
	if err == nil {
		t.Fatal("bad status")
	}
	_, err = mapDataValue(TagView{Tag: core.Tag{ID: "x", DataType: "nope"}},
		&ua.DataValue{Status: ua.StatusOK, Value: bv}, now)
	if err == nil {
		t.Fatal("unsupported type")
	}
}

func TestNumericToTimeAndParse(t *testing.T) {
	if !numericToTime(0).IsZero() {
		t.Fatal("zero")
	}
	unix := time.Unix(1_700_000_000, 0).UTC()
	if !numericToTime(1_700_000_000).Equal(unix) {
		t.Fatalf("unix sec")
	}
	if !numericToTime(1_700_000_000_000).Equal(time.UnixMilli(1_700_000_000_000).UTC()) {
		t.Fatalf("unix milli")
	}
	tm, err := parseDateTimeString("2026-01-02")
	if err != nil || tm.Year() != 2026 {
		t.Fatalf("%v %v", tm, err)
	}
	if _, err := parseDateTimeString(""); err == nil {
		t.Fatal("empty")
	}
	if _, err := parseDateTimeString("not-a-date"); err == nil {
		t.Fatal("garbage")
	}
}
