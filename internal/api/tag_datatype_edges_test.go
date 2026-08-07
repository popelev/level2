package api

import (
	"context"
	"testing"

	"github.com/popelev/level2/internal/core"
	devruntime "github.com/popelev/level2/internal/runtime"
)

func TestResolveTagDataType_NormalizeWithoutOPC(t *testing.T) {
	s := &Server{}
	tag := &core.Tag{NodeID: "ns=4;i=1", DataType: "float"}
	s.resolveTagDataType(context.Background(), "plc", tag)
	if tag.DataType != core.ValueFloat64 {
		t.Fatalf("got %q", tag.DataType)
	}
	s.resolveTagDataType(context.Background(), "plc", nil) // no panic
	empty := &core.Tag{DataType: ""}
	s.resolveTagDataType(context.Background(), "plc", empty)
	if empty.DataType != "" {
		t.Fatalf("empty without hub should stay empty, got %q", empty.DataType)
	}
}

func TestResolveEmptyDataTypesBatch_GuessWithoutDriver(t *testing.T) {
	hub := devruntime.NewHub(nil, true)
	s := &Server{DevHub: hub}
	tags := []core.Tag{
		{ID: "rValueOut", NodeID: "ns=4;i=1", DataType: ""},
		{ID: "keep", NodeID: "ns=4;i=2", DataType: core.ValueBool},
	}
	s.resolveEmptyDataTypesBatch(context.Background(), "missing-device", tags)
	if tags[0].DataType == "" {
		t.Fatal("expected guess for empty datatype")
	}
	if tags[1].DataType != core.ValueBool {
		t.Fatalf("explicit type overwritten: %q", tags[1].DataType)
	}
}
