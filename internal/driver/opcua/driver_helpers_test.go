package opcua

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
)

func TestNewConnectedAndDisconnectNil(t *testing.T) {
	d := New(core.Device{ID: "plc", Endpoint: "opc.tcp://x"}, nil)
	if d.Connected() {
		t.Fatal("expected disconnected")
	}
	if err := d.Disconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d2 := New(core.Device{ID: "plc2"}, log)
	if d2.log == nil {
		t.Fatal("log")
	}
}

func TestEmitBadSamples(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan core.Sample, 4)
	tags := []TagView{
		{Tag: core.Tag{ID: "a"}},
		{Tag: core.Tag{ID: "b"}},
	}
	emitBadSamples(ctx, tags, out)
	for _, id := range []string{"a", "b"} {
		select {
		case s := <-out:
			if s.TagID != id || s.Quality != core.QualityBad {
				t.Fatalf("got %#v want Bad %s", s, id)
			}
		default:
			t.Fatalf("missing sample for %s", id)
		}
	}
	cancel()
	// Cancelled ctx must not block.
	emitBadSamples(ctx, tags, make(chan core.Sample))
}

	inc := diag.NewIncidentTracker(100, 0)
	diag.SetDefaultIncidents(inc)
	t.Cleanup(func() { diag.SetDefaultIncidents(diag.NewIncidentTracker(100, 0)) })

	d := New(core.Device{ID: "dev1"}, nil)
	d.alive.Store(true)
	d.markDown(true)
	if d.Connected() {
		t.Fatal("should be down")
	}
	if got := inc.Count(diag.IncidentOPCDisconnect, 0); got != 1 {
		t.Fatalf("count=%d", got)
	}
	// already down — no second record
	d.markDown(true)
	if got := inc.Count(diag.IncidentOPCDisconnect, 0); got != 1 {
		t.Fatalf("second markDown counted: %d", got)
	}
	d.alive.Store(true)
	d.markDown(false)
	if got := inc.Count(diag.IncidentOPCDisconnect, 0); got != 1 {
		t.Fatalf("record=false should not count: %d", got)
	}
}

func TestFormatNodeID_Variants(t *testing.T) {
	cases := []struct {
		name string
		n    *ua.NodeID
		want string
	}{
		{"nil", nil, ""},
		{"numeric", ua.NewNumericNodeID(4, 4208), "ns=4;i=4208"},
		{"string", ua.NewStringNodeID(2, "Foo.Bar"), "ns=2;s=Foo.Bar"},
		{"bytes", ua.NewByteStringNodeID(3, []byte{0x01, 0x02}), ""},
	}
	for _, tc := range cases {
		got := formatNodeID(tc.n)
		if tc.name == "bytes" {
			if !strings.HasPrefix(got, "ns=3;b=") {
				t.Fatalf("bytes got %q", got)
			}
			continue
		}
		if got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestExpandFromTree_EmptyAndNonLeaf(t *testing.T) {
	if got := ExpandFromTree("p", nil); len(got) != 0 {
		t.Fatalf("%#v", got)
	}
	nodes := []core.BrowseNode{
		{NodeID: "ns=4;i=1", BrowseName: "Folder", IsLeaf: false},
		{NodeID: "ns=4;i=2", BrowseName: "rValueOut", IsLeaf: true},
		{NodeID: "ns=4;i=3", BrowseName: "sUnit", IsLeaf: true},
	}
	got := ExpandFromTree("mp", nodes)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].DataType != core.ValueFloat64 || got[1].DataType != core.ValueString {
		t.Fatalf("%#v", got)
	}
}

func TestToUANodeID_NumericErrors(t *testing.T) {
	if _, err := toUANodeID(core.ParsedNodeID{Namespace: 1, IdentifierType: "i", Identifier: "x"}); err == nil {
		t.Fatal("bad int")
	}
	if _, err := toUANodeID(core.ParsedNodeID{Namespace: 1, IdentifierType: "b", Identifier: "x"}); err == nil {
		t.Fatal("bytes unsupported")
	}
}

func TestPrepareTags_NSU(t *testing.T) {
	got, err := PrepareTags([]core.Tag{{
		ID: "t", NodeID: "nsu=http://example;i=42", DataType: core.ValueFloat64,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Parsed.NamespaceURI != "http://example" || got[0].Parsed.Identifier != "42" {
		t.Fatalf("%#v", got[0].Parsed)
	}
}
