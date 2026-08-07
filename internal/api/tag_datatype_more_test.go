package api

import (
	"context"
	"testing"

	"github.com/popelev/level2/internal/core"
	devruntime "github.com/popelev/level2/internal/runtime"
)

// onlineNonOPC is Connected but does not implement core.DataTypeResolver — covers interface miss paths.
type onlineNonOPC struct{}

func (onlineNonOPC) Connect(context.Context) error    { return nil }
func (onlineNonOPC) Disconnect(context.Context) error { return nil }
func (onlineNonOPC) Connected() bool                  { return true }
func (onlineNonOPC) Subscribe(context.Context, []core.Tag, chan<- core.Sample) error {
	return nil
}

func TestResolveTagDataType_ConnectedNonOPC(t *testing.T) {
	hub := devruntime.NewHub(nil, false)
	hub.InjectDriver(core.Device{ID: "plc"}, onlineNonOPC{}, nil)
	s := &Server{DevHub: hub}

	tag := &core.Tag{ID: "tank", NodeID: "ns=4;i=1", DataType: ""}
	s.resolveTagDataType(context.Background(), "plc", tag)
	if tag.DataType != "" {
		t.Fatalf("non-opcua connected should not invent type, got %q", tag.DataType)
	}

	// invalid alias stays until normalize fails ValidValueType path with empty after normalize
	tag2 := &core.Tag{ID: "x", NodeID: "ns=4;i=2", DataType: "nope"}
	s.resolveTagDataType(context.Background(), "plc", tag2)
	if tag2.DataType != "nope" {
		t.Fatalf("invalid type should remain when OPC resolve unavailable, got %q", tag2.DataType)
	}

	s.resolveTagDataType(context.Background(), "missing", &core.Tag{NodeID: "ns=1;i=1", DataType: ""})
}

func TestResolveEmptyDataTypesBatch_ConnectedNonOPCAndEmpty(t *testing.T) {
	hub := devruntime.NewHub(nil, false)
	hub.InjectDriver(core.Device{ID: "plc"}, onlineNonOPC{}, nil)
	s := &Server{DevHub: hub}

	tags := []core.Tag{
		{ID: "bEnable", NodeID: "ns=4;i=1", DataType: ""},
		{ID: "rLevel", NodeID: "ns=4;i=2", DataType: ""},
		{ID: "keep", NodeID: "ns=4;i=3", DataType: core.ValueFloat64},
	}
	s.resolveEmptyDataTypesBatch(context.Background(), "plc", tags)
	if tags[0].DataType == "" || tags[1].DataType == "" {
		t.Fatalf("expected GuessDataType for non-opcua connected: %#v", tags)
	}
	if tags[2].DataType != core.ValueFloat64 {
		t.Fatalf("kept type overwritten: %q", tags[2].DataType)
	}

	s.resolveEmptyDataTypesBatch(context.Background(), "plc", nil)
	s.resolveEmptyDataTypesBatch(context.Background(), "plc", []core.Tag{{ID: "a", DataType: core.ValueBool}})
}
