package opcua

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

func TestConnect_FailsWithoutServer(t *testing.T) {
	d := New(core.Device{ID: "d", Endpoint: "opc.tcp://127.0.0.1:1"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := d.Connect(ctx)
	if err == nil {
		_ = d.Disconnect(ctx)
		t.Fatal("expected connect failure")
	}
	if !strings.Contains(err.Error(), "opcua") {
		t.Fatalf("%v", err)
	}
}

func TestConnect_AlreadyHasClient(t *testing.T) {
	d := driverWithSession(&mockSession{})
	if err := d.Connect(context.Background()); err != nil {
		t.Fatalf("already connected must no-op: %v", err)
	}
	if !d.Connected() {
		t.Fatal("expected still connected")
	}
}

func TestSetConnectedForTest(t *testing.T) {
	d := New(core.Device{ID: "d"}, nil)
	d.SetConnectedForTest(true)
	if !d.Connected() {
		t.Fatal("expected connected")
	}
	d.SetConnectedForTest(false)
	if d.Connected() {
		t.Fatal("expected disconnected")
	}
}

func TestSubscribe_NoEnabledTags(t *testing.T) {
	d := New(core.Device{ID: "d"}, nil)
	err := d.Subscribe(context.Background(), []core.Tag{
		{ID: "a", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: false},
	}, make(chan core.Sample))
	if err == nil || !strings.Contains(err.Error(), "no enabled tags") {
		t.Fatalf("%v", err)
	}
	_, err = PrepareTags([]core.Tag{{ID: "bad", NodeID: "not-a-node", DataType: core.ValueFloat64}})
	if err == nil {
		t.Fatal("bad node")
	}
}

func TestSubscribe_CancelBeforePoll(t *testing.T) {
	d := New(core.Device{ID: "d", Endpoint: "opc.tcp://127.0.0.1:1"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := make(chan core.Sample, 4)
	err := d.Subscribe(ctx, []core.Tag{
		{ID: "a", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 50},
	}, out)
	if err != context.Canceled {
		t.Fatalf("want canceled, got %v", err)
	}
}

func TestExpandFromRefs_OfflineEdges(t *testing.T) {
	d := &Driver{}
	var out []core.ExpandedTag
	if err := d.expandFromRefs(context.Background(), nil, "p", "", 5, 3, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("%v", out)
	}
	// nil / incomplete refs → empty children → nil
	refs := []*ua.ReferenceDescription{
		nil,
		{NodeID: nil},
		{NodeID: &ua.ExpandedNodeID{}},
	}
	if err := d.expandFromRefs(context.Background(), refs, "p", "root", 0, 4, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("incomplete refs must yield no tags: %#v", out)
	}
}

func TestNamespaceIndex_CacheHit(t *testing.T) {
	d := &Driver{nsCache: map[string]uint16{"http://ex": 7}}
	idx, err := d.namespaceIndex(context.Background(), "http://ex")
	if err != nil || idx != 7 {
		t.Fatalf("%d %v", idx, err)
	}
	n, err := d.toUANodeID(context.Background(), core.ParsedNodeID{
		NamespaceURI: "http://ex", IdentifierType: "i", Identifier: "9",
	})
	if err != nil || n.Namespace() != 7 || n.IntID() != 9 {
		t.Fatalf("%v %v", n, err)
	}
}

func TestBrowseChildren_NotConnectedAndBadNode(t *testing.T) {
	d := New(core.Device{ID: "d"}, nil)
	if _, err := d.BrowseChildren(context.Background(), "ns=4;i=1"); err == nil {
		t.Fatal("not connected")
	}
	// still not connected — ParseNodeID not reached after nil client
	if _, err := d.BrowseChildren(context.Background(), "bad"); err == nil {
		t.Fatal("not connected")
	}
	if _, _, err := d.ProbeNode(context.Background(), "bad-node"); err == nil {
		t.Fatal("not connected probe")
	}
}

func TestPollOnce_MarkDownPathViaSubscribeTick(t *testing.T) {
	// Connected mock that fails Read → Subscribe pollOnce error → emitBadSamples.
	s := &mockSession{readFn: func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
		return nil, errors.New("read fail")
	}}
	d := driverWithSession(s)
	d.device.Endpoint = "opc.tcp://127.0.0.1:1" // reconnect will fail fast-ish
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	out := make(chan core.Sample, 8)
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Subscribe(ctx, []core.Tag{
			{ID: "a", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 20},
		}, out)
	}()
	deadline := time.After(800 * time.Millisecond)
	var bad core.Sample
	sawBad := false
	for !sawBad {
		select {
		case s := <-out:
			if s.Quality == core.QualityBad {
				bad = s
				sawBad = true
			}
		case <-deadline:
			t.Fatal("expected Bad sample after poll failure")
		}
	}
	if bad.TagID != "a" {
		t.Fatalf("bad sample %#v", bad)
	}
	<-errCh // Subscribe exits on ctx cancel
}
