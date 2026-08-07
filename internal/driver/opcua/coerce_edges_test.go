package opcua

import (
	"context"
	"strings"
	"testing"

	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

func TestWriteStatusError(t *testing.T) {
	var nilErr *WriteStatusError
	if got := nilErr.Error(); got != "opc write status error" {
		t.Fatalf("nil: %q", got)
	}
	e := &WriteStatusError{Status: ua.StatusBad}
	got := e.Error()
	if got == "" || got == "opc write status error" {
		t.Fatalf("non-nil Status.Error: %q", got)
	}
}

func TestCoerceWriteValue_UnsupportedAndAliases(t *testing.T) {
	if _, err := CoerceWriteValue("blob", 1); err == nil {
		t.Fatal("unsupported")
	}
	got, err := CoerceWriteValue(core.ValueBool, float64(0))
	if err != nil || got != false {
		t.Fatalf("bool 0: %#v %v", got, err)
	}
}

func TestWriteValue_NotConnected(t *testing.T) {
	d := New(core.Device{ID: "x"}, nil)
	err := d.WriteValue(context.Background(), core.Tag{ID: "t", NodeID: "ns=4;i=1"}, true)
	if err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("got %v", err)
	}
}

func TestReadValue_NotConnected(t *testing.T) {
	d := New(core.Device{ID: "x"}, nil)
	_, err := d.ReadValue(context.Background(), core.Tag{ID: "t", NodeID: "ns=4;i=1", DataType: core.ValueBool})
	if err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("got %v", err)
	}
}
