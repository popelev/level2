package api

import (
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestValidateTagFields(t *testing.T) {
	tg := core.Tag{ID: "t", NodeID: "ns=4;i=1", DataType: "float", IntervalMs: 0}
	if err := validateTagFields(&tg); err != nil {
		t.Fatal(err)
	}
	if tg.IntervalMs != 1000 || tg.DataType != core.ValueFloat64 {
		t.Fatalf("%#v", tg)
	}

	tg = core.Tag{DataType: "nope", IntervalMs: 50}
	err := validateTagFields(&tg)
	if err == nil {
		t.Fatal("expected unsupported datatype")
	}
	if _, ok := err.(errUnsupportedDataType); !ok {
		t.Fatalf("want errUnsupportedDataType, got %T %v", err, err)
	}
}
