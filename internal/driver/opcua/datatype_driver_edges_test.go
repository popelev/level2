package opcua

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestGuessDataType_EnableAutoRunActiveDate(t *testing.T) {
	cases := map[string]core.ValueType{
		"enablePump":   core.ValueBool,
		"EnableValve":  core.ValueBool,
		"motor_auto":   core.ValueBool,
		"pump_run":     core.ValueBool,
		"line_active":  core.ValueBool,
		"Date":         core.ValueDateTime,
		"Tank.Date":    core.ValueDateTime,
		"boolFlag":     core.ValueBool,
		"item_count":   core.ValueInt64,
	}
	for name, want := range cases {
		if got := GuessDataType(name); got != want {
			t.Fatalf("%q: got %q want %q", name, got, want)
		}
	}
}

func TestBrowseNameHint_NodeIDFallback(t *testing.T) {
	got := browseNameHint(core.ExpandedTag{NodeID: "ns=4;i=9"})
	if got != "ns=4;i=9" {
		t.Fatalf("%q", got)
	}
}

func TestApplyDataTypesFromOPC_HintFromNodeID(t *testing.T) {
	d := &Driver{}
	tags := []core.Tag{{
		ID:       "",
		NodeID:   "ns=4;s=sUnit",
		DataType: core.ValueFloat64,
	}}
	ApplyDataTypesFromOPC(context.Background(), d, tags)
	// Hint falls back to NodeID which contains sUnit → string.
	if tags[0].DataType != core.ValueString {
		t.Fatalf("got %q", tags[0].DataType)
	}
}

func TestReadOPCDataType_DisconnectedError(t *testing.T) {
	d := &Driver{}
	_, err := d.readOPCDataType(context.Background(), "ns=4;i=1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFillExpandedDataTypes_Empty(t *testing.T) {
	d := &Driver{}
	d.fillExpandedDataTypes(context.Background(), nil, func(string, int, int) {
		t.Fatal("should not call")
	})
}

func TestDriverToUANodeID_Offline(t *testing.T) {
	d := New(core.Device{ID: "d"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	n, err := d.toUANodeID(ctx, core.ParsedNodeID{Namespace: 4, IdentifierType: "i", Identifier: "42"})
	if err != nil || n.IntID() != 42 || n.Namespace() != 4 {
		t.Fatalf("%v %v", n, err)
	}
	s, err := d.toUANodeID(ctx, core.ParsedNodeID{Namespace: 2, IdentifierType: "s", Identifier: "Foo"})
	if err != nil || s.StringID() != "Foo" {
		t.Fatalf("%v %v", s, err)
	}
	if _, err := d.toUANodeID(ctx, core.ParsedNodeID{IdentifierType: "i", Identifier: "x"}); err == nil {
		t.Fatal("bad int")
	}
	if _, err := d.toUANodeID(ctx, core.ParsedNodeID{IdentifierType: "g", Identifier: "x"}); err == nil {
		t.Fatal("guid")
	}
	// NamespaceURI requires connected client.
	_, err = d.toUANodeID(ctx, core.ParsedNodeID{NamespaceURI: "http://ex", IdentifierType: "i", Identifier: "1"})
	if err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("nsu: %v", err)
	}
}

func TestNamespaceIndex_NotConnected(t *testing.T) {
	d := &Driver{}
	_, err := d.namespaceIndex(context.Background(), "http://ex")
	if err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("%v", err)
	}
}

func TestPollOnce_EmptyAndPollBatchOffline(t *testing.T) {
	d := New(core.Device{ID: "d"}, nil)
	ctx := context.Background()
	if err := d.pollOnce(ctx, nil, make(chan core.Sample)); err != nil {
		t.Fatal(err)
	}
	err := d.pollBatch(ctx, []TagView{{Tag: core.Tag{ID: "a"}}}, make(chan core.Sample, 1))
	if err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("%v", err)
	}
}

func TestExpandStructureWithProgress_DefaultDepthAndPrefix(t *testing.T) {
	d := New(core.Device{ID: "d"}, nil)
	// Not connected → expandWalk → browseRefsAt fails.
	_, err := d.ExpandStructureWithProgress(context.Background(), "ns=4;i=1", "", 0, func(phase string, done, total int) {
		t.Fatalf("progress %s %d/%d", phase, done, total)
	})
	if err == nil {
		t.Fatal("expected not connected")
	}
}
