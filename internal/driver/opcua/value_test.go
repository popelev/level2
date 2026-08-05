package opcua

import (
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

func TestMapFloat(t *testing.T) {
	tag := TagView{Tag: core.Tag{ID: "t", DataType: core.ValueFloat64}}
	v, err := ua.NewVariant(float32(90.5))
	if err != nil {
		t.Fatal(err)
	}
	dv := &ua.DataValue{Status: ua.StatusOK, Value: v}
	s, err := mapDataValue(tag, dv, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if s.ValueNum == nil || *s.ValueNum != 90.5 {
		t.Fatalf("got %#v", s.ValueNum)
	}
}

func TestMapString(t *testing.T) {
	tag := TagView{Tag: core.Tag{ID: "u", DataType: core.ValueString}}
	v, err := ua.NewVariant("%")
	if err != nil {
		t.Fatal(err)
	}
	dv := &ua.DataValue{Status: ua.StatusOK, Value: v}
	s, err := mapDataValue(tag, dv, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if s.ValueText == nil || *s.ValueText != "%" {
		t.Fatalf("got %#v", s.ValueText)
	}
}

func TestMapStructureStatus(t *testing.T) {
	tag := TagView{Tag: core.Tag{ID: "s", DataType: core.ValueFloat64}}
	dv := &ua.DataValue{Status: ua.StatusCode(0x80110000)} // BadDataTypeIDUnknown
	_, err := mapDataValue(tag, dv, time.Now().UTC())
	if err == nil {
		t.Fatal("expected structure error")
	}
}
