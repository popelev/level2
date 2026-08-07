package opcua

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

// mockSession implements uaSession for offline unit tests (no live OPC).
type mockSession struct {
	mu sync.Mutex

	browseFn     func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error)
	browseNextFn func(context.Context, *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error)
	readFn       func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error)
	writeFn      func(context.Context, *ua.WriteRequest) (*ua.WriteResponse, error)
	closeFn      func(context.Context) error
	nsFn         func(context.Context) ([]string, error)

	closeCalls int
}

func (m *mockSession) Browse(ctx context.Context, req *ua.BrowseRequest) (*ua.BrowseResponse, error) {
	if m.browseFn != nil {
		return m.browseFn(ctx, req)
	}
	return nil, errors.New("browse not stubbed")
}

func (m *mockSession) BrowseNext(ctx context.Context, req *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error) {
	if m.browseNextFn != nil {
		return m.browseNextFn(ctx, req)
	}
	return nil, errors.New("browseNext not stubbed")
}

func (m *mockSession) Read(ctx context.Context, req *ua.ReadRequest) (*ua.ReadResponse, error) {
	if m.readFn != nil {
		return m.readFn(ctx, req)
	}
	return nil, errors.New("read not stubbed")
}

func (m *mockSession) Write(ctx context.Context, req *ua.WriteRequest) (*ua.WriteResponse, error) {
	if m.writeFn != nil {
		return m.writeFn(ctx, req)
	}
	return nil, errors.New("write not stubbed")
}

func (m *mockSession) Close(ctx context.Context) error {
	m.mu.Lock()
	m.closeCalls++
	m.mu.Unlock()
	if m.closeFn != nil {
		return m.closeFn(ctx)
	}
	return nil
}

func (m *mockSession) NamespaceArray(ctx context.Context) ([]string, error) {
	if m.nsFn != nil {
		return m.nsFn(ctx)
	}
	return nil, errors.New("ns not stubbed")
}

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func driverWithSession(s uaSession) *Driver {
	d := New(core.Device{ID: "dev", Endpoint: "opc.tcp://mock", PollConcurrency: 2}, discardLog())
	d.client = s
	d.alive.Store(true)
	return d
}

func refVar(ns uint16, i uint32, name string) *ua.ReferenceDescription {
	return &ua.ReferenceDescription{
		NodeID:      &ua.ExpandedNodeID{NodeID: ua.NewNumericNodeID(ns, i)},
		BrowseName:  &ua.QualifiedName{Name: name},
		DisplayName: ua.NewLocalizedText(name),
		NodeClass:   ua.NodeClassVariable,
	}
}

func refObj(ns uint16, i uint32, name string) *ua.ReferenceDescription {
	r := refVar(ns, i, name)
	r.NodeClass = ua.NodeClassObject
	return r
}

func okBrowse(refs ...*ua.ReferenceDescription) *ua.BrowseResponse {
	return &ua.BrowseResponse{Results: []*ua.BrowseResult{{
		StatusCode: ua.StatusOK,
		References: refs,
	}}}
}

func TestConnect_AlreadyConnected(t *testing.T) {
	d := driverWithSession(&mockSession{})
	if err := d.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDisconnect_ClosesClient(t *testing.T) {
	s := &mockSession{}
	d := driverWithSession(s)
	if err := d.Disconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.closeCalls != 1 {
		t.Fatalf("closeCalls=%d", s.closeCalls)
	}
	if d.client != nil {
		t.Fatal("client should be nil")
	}
}

func TestNamespaceIndex_FromArray(t *testing.T) {
	s := &mockSession{nsFn: func(context.Context) ([]string, error) {
		return []string{"http://opcfoundation.org/UA/", "http://ex"}, nil
	}}
	d := driverWithSession(s)
	idx, err := d.namespaceIndex(context.Background(), "http://ex")
	if err != nil || idx != 1 {
		t.Fatalf("%d %v", idx, err)
	}
	// cache hit
	idx, err = d.namespaceIndex(context.Background(), "http://ex")
	if err != nil || idx != 1 {
		t.Fatalf("cache %d %v", idx, err)
	}
	_, err = d.namespaceIndex(context.Background(), "http://missing")
	if err == nil {
		t.Fatal("missing uri")
	}

	s2 := &mockSession{nsFn: func(context.Context) ([]string, error) {
		return nil, errors.New("ns fail")
	}}
	d2 := driverWithSession(s2)
	if _, err := d2.namespaceIndex(context.Background(), "x"); err == nil {
		t.Fatal("ns fail")
	}
}

func TestBrowseChildren_MockTree(t *testing.T) {
	leaf := refVar(4, 2, "rValueOut")
	folder := refObj(4, 3, "Folder")
	structVar := refVar(4, 4, "UDT")
	junk := &ua.ReferenceDescription{NodeID: nil}

	browseCalls := 0
	s := &mockSession{
		browseFn: func(_ context.Context, req *ua.BrowseRequest) (*ua.BrowseResponse, error) {
			browseCalls++
			if len(req.NodesToBrowse) == 0 {
				return nil, errors.New("empty")
			}
			nid := req.NodesToBrowse[0].NodeID
			switch {
			case nid.IntID() == 1:
				return okBrowse(junk, leaf, folder, structVar), nil
			case nid.IntID() == 2: // leaf — no children
				return okBrowse(), nil
			case nid.IntID() == 4: // structure — has kids
				return okBrowse(refVar(4, 5, "field")), nil
			default:
				return okBrowse(), nil
			}
		},
		readFn: func(_ context.Context, req *ua.ReadRequest) (*ua.ReadResponse, error) {
			out := make([]*ua.DataValue, len(req.NodesToRead))
			for i := range req.NodesToRead {
				v, _ := ua.NewVariant(ua.NewNumericNodeID(0, id.Double))
				out[i] = &ua.DataValue{Status: ua.StatusOK, Value: v}
			}
			return &ua.ReadResponse{Results: out}, nil
		},
	}
	d := driverWithSession(s)
	nodes, err := d.BrowseChildren(context.Background(), "ns=4;i=1")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("nodes=%d %#v", len(nodes), nodes)
	}
	var leafN *core.BrowseNode
	for i := range nodes {
		if nodes[i].BrowseName == "rValueOut" {
			leafN = &nodes[i]
		}
	}
	if leafN == nil || !leafN.IsLeaf || leafN.DataType != string(core.ValueFloat64) || leafN.NodeID != "ns=4;i=2" {
		t.Fatalf("leaf %#v", leafN)
	}
	byName := map[string]core.BrowseNode{}
	for _, n := range nodes {
		byName[n.BrowseName] = n
	}
	if f, ok := byName["Folder"]; !ok || f.IsLeaf || f.NodeClass == "" {
		t.Fatalf("folder %#v", byName["Folder"])
	}
	if u, ok := byName["UDT"]; !ok || u.IsLeaf {
		t.Fatalf("UDT structure must not be leaf: %#v", byName["UDT"])
	}

	if _, err := d.BrowseChildren(context.Background(), "bad-node"); err == nil {
		t.Fatal("parse")
	}
}

func TestBrowseChildren_BrowseError(t *testing.T) {
	s := &mockSession{browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
		return nil, errors.New("boom")
	}}
	d := driverWithSession(s)
	if _, err := d.BrowseChildren(context.Background(), "ns=4;i=1"); err == nil {
		t.Fatal("expected err")
	}
}

func TestProbeNode_MockVariants(t *testing.T) {
	// gopcua Variant does not accept ua.NodeClass directly; stacks often return int32.
	mk := func(val any, status ua.StatusCode) *mockSession {
		return &mockSession{readFn: func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
			v, err := ua.NewVariant(val)
			if err != nil {
				return nil, err
			}
			return &ua.ReadResponse{Results: []*ua.DataValue{{Status: status, Value: v}}}, nil
		}}
	}

	d := driverWithSession(mk(int32(ua.NodeClassVariable), ua.StatusOK))
	ex, leaf, err := d.ProbeNode(context.Background(), "ns=4;i=9")
	if err != nil || !ex || !leaf {
		t.Fatalf("%v %v %v", ex, leaf, err)
	}

	d = driverWithSession(mk(int32(ua.NodeClassObject), ua.StatusOK))
	ex, leaf, err = d.ProbeNode(context.Background(), "ns=4;i=9")
	if err != nil || !ex || leaf {
		t.Fatalf("obj %v %v %v", ex, leaf, err)
	}

	// Direct NodeClass via crafted Value() path: decode as NodeClass when type matches.
	d = driverWithSession(&mockSession{readFn: func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
		v, err := ua.NewVariant(int32(ua.NodeClassVariable))
		if err != nil {
			return nil, err
		}
		return &ua.ReadResponse{Results: []*ua.DataValue{{Status: ua.StatusOK, Value: v}}}, nil
	}})
	ex, leaf, err = d.ProbeNode(context.Background(), "ns=4;i=9")
	if err != nil || !ex || !leaf {
		t.Fatalf("int32 leaf %v %v %v", ex, leaf, err)
	}

	d = driverWithSession(mk("weird", ua.StatusOK))
	ex, leaf, err = d.ProbeNode(context.Background(), "ns=4;i=9")
	if err != nil || !ex || leaf {
		t.Fatalf("weird %v %v %v", ex, leaf, err)
	}

	d = driverWithSession(mk(int32(ua.NodeClassVariable), ua.StatusBad))
	ex, leaf, err = d.ProbeNode(context.Background(), "ns=4;i=9")
	if err != nil || ex || leaf {
		t.Fatalf("bad status %v %v %v", ex, leaf, err)
	}

	d = driverWithSession(&mockSession{readFn: func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
		return &ua.ReadResponse{}, nil
	}})
	ex, leaf, err = d.ProbeNode(context.Background(), "ns=4;i=9")
	if err != nil || ex {
		t.Fatalf("empty %v %v %v", ex, leaf, err)
	}

	d = driverWithSession(&mockSession{readFn: func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
		return nil, errors.New("transport")
	}})
	if _, _, err := d.ProbeNode(context.Background(), "ns=4;i=9"); err == nil {
		t.Fatal("transport")
	}

	d = driverWithSession(&mockSession{})
	if _, _, err := d.ProbeNode(context.Background(), "not-a-node"); err == nil {
		t.Fatal("parse")
	}
}

func TestExpandStructure_MockLeaves(t *testing.T) {
	// Parent Object i=10 → child Variable leaf i=11 (no grandkids) + Object i=12 → Variable i=13
	s := &mockSession{
		browseFn: func(_ context.Context, req *ua.BrowseRequest) (*ua.BrowseResponse, error) {
			n := len(req.NodesToBrowse)
			results := make([]*ua.BrowseResult, n)
			for i, desc := range req.NodesToBrowse {
				var refs []*ua.ReferenceDescription
				switch desc.NodeID.IntID() {
				case 10:
					refs = []*ua.ReferenceDescription{
						refVar(4, 11, "rValueOut"),
						refObj(4, 12, "Nested"),
					}
				case 11:
					refs = nil // leaf
				case 12:
					refs = []*ua.ReferenceDescription{refVar(4, 13, "sUnit")}
				case 13:
					refs = nil
				}
				results[i] = &ua.BrowseResult{StatusCode: ua.StatusOK, References: refs}
			}
			return &ua.BrowseResponse{Results: results}, nil
		},
		readFn: func(_ context.Context, req *ua.ReadRequest) (*ua.ReadResponse, error) {
			out := make([]*ua.DataValue, len(req.NodesToRead))
			for i, n := range req.NodesToRead {
				tid := uint32(id.Double)
				if n.NodeID != nil && n.NodeID.IntID() == 13 {
					tid = id.String
				}
				v, _ := ua.NewVariant(ua.NewNumericNodeID(0, tid))
				out[i] = &ua.DataValue{Status: ua.StatusOK, Value: v}
			}
			return &ua.ReadResponse{Results: out}, nil
		},
	}
	d := driverWithSession(s)
	var phases []string
	tags, err := d.ExpandStructureWithProgress(context.Background(), "ns=4;i=10", "mp", 8, func(phase string, done, total int) {
		phases = append(phases, fmt.Sprintf("%s:%d/%d", phase, done, total))
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 {
		t.Fatalf("tags=%d %#v", len(tags), tags)
	}
	byPath := map[string]core.ExpandedTag{}
	for _, tg := range tags {
		byPath[tg.BrowsePath] = tg
	}
	if byPath["rValueOut"].DataType != core.ValueFloat64 {
		t.Fatalf("%#v", byPath["rValueOut"])
	}
	if byPath["Nested.sUnit"].DataType != core.ValueString || byPath["Nested.sUnit"].NodeID != "ns=4;i=13" {
		t.Fatalf("%#v", byPath["Nested.sUnit"])
	}
	if byPath["rValueOut"].NodeID != "ns=4;i=11" {
		t.Fatalf("%#v", byPath["rValueOut"])
	}
	if len(phases) < 2 {
		t.Fatalf("phases=%v", phases)
	}

	// depth already past max → empty, no error (ExpandStructure remaps maxDepth<=0 to 16)
	var limited []core.ExpandedTag
	if err := d.expandWalk(context.Background(), "ns=4;i=10", "p", "", 5, 3, &limited); err != nil {
		t.Fatal(err)
	}
	if len(limited) != 0 {
		t.Fatalf("depth>maxDepth want empty, got %#v", limited)
	}
}

func TestBrowseRefsMulti_BatchesAndNotConnected(t *testing.T) {
	calls := 0
	s := &mockSession{browseFn: func(_ context.Context, req *ua.BrowseRequest) (*ua.BrowseResponse, error) {
		calls++
		results := make([]*ua.BrowseResult, len(req.NodesToBrowse))
		for i := range results {
			results[i] = &ua.BrowseResult{StatusCode: ua.StatusOK}
		}
		return &ua.BrowseResponse{Results: results}, nil
	}}
	d := driverWithSession(s)
	ids := make([]*ua.NodeID, expandBrowseBatch+3)
	for i := range ids {
		ids[i] = ua.NewNumericNodeID(4, uint32(i+1))
	}
	got, err := d.browseRefsMulti(context.Background(), ids)
	if err != nil || len(got) != len(ids) || calls != 2 {
		t.Fatalf("len=%d calls=%d err=%v", len(got), calls, err)
	}
	for i, refs := range got {
		if len(refs) != 0 {
			t.Fatalf("slot %d want empty refs, got %#v", i, refs)
		}
	}

	d2 := New(core.Device{ID: "x"}, nil)
	if _, err := d2.browseRefsMulti(context.Background(), ids[:1]); err == nil {
		t.Fatal("not connected")
	}
}

func TestReadOPCDataTypesBatch_MockRead(t *testing.T) {
	s := &mockSession{readFn: func(_ context.Context, req *ua.ReadRequest) (*ua.ReadResponse, error) {
		out := make([]*ua.DataValue, len(req.NodesToRead))
		for i := range req.NodesToRead {
			v, _ := ua.NewVariant(ua.NewNumericNodeID(0, id.Boolean))
			out[i] = &ua.DataValue{Status: ua.StatusOK, Value: v}
		}
		return &ua.ReadResponse{Results: out}, nil
	}}
	d := driverWithSession(s)
	var chunks []int
	types := d.readOPCDataTypesBatch(context.Background(), []string{"ns=4;i=1", "bad", "ns=4;s=x"}, func(done, total int) {
		chunks = append(chunks, done)
	})
	if types[0] != core.ValueBool || types[1] != "" || types[2] != core.ValueBool {
		t.Fatalf("%#v", types)
	}
	if len(chunks) == 0 {
		t.Fatal("onChunk")
	}
	dt, err := d.readOPCDataType(context.Background(), "ns=4;i=1")
	if err != nil || dt != core.ValueBool {
		t.Fatalf("%q %v", dt, err)
	}

	// Read transport error → empty mapped types
	s2 := &mockSession{readFn: func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
		return nil, errors.New("fail")
	}}
	d2 := driverWithSession(s2)
	types = d2.readOPCDataTypesBatch(context.Background(), []string{"ns=4;i=1"}, nil)
	if types[0] != "" {
		t.Fatalf("%#v", types)
	}

	// Non-NodeID / bad status / nil result skipped
	s3 := &mockSession{readFn: func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
		vStr, _ := ua.NewVariant("nope")
		return &ua.ReadResponse{Results: []*ua.DataValue{
			nil,
			{Status: ua.StatusBad, Value: vStr},
			{Status: ua.StatusOK, Value: vStr},
			{Status: ua.StatusOK, Value: nil},
		}}, nil
	}}
	d3 := driverWithSession(s3)
	types = d3.readOPCDataTypesBatch(context.Background(), []string{"ns=4;i=1", "ns=4;i=2", "ns=4;i=3", "ns=4;i=4"}, nil)
	for _, tpe := range types {
		if tpe != "" {
			t.Fatalf("%#v", types)
		}
	}
}

func TestPollBatch_AndPollOnceWorkers(t *testing.T) {
	s := &mockSession{readFn: func(_ context.Context, req *ua.ReadRequest) (*ua.ReadResponse, error) {
		out := make([]*ua.DataValue, len(req.NodesToRead))
		for i := range out {
			v, _ := ua.NewVariant(float64(i + 1))
			out[i] = &ua.DataValue{Status: ua.StatusOK, Value: v, SourceTimestamp: time.Now().UTC()}
		}
		return &ua.ReadResponse{Results: out}, nil
	}}
	d := driverWithSession(s)
	d.device.PollConcurrency = 4

	views := make([]TagView, 0, 120)
	for i := 0; i < 120; i++ {
		views = append(views, TagView{
			Tag:    core.Tag{ID: fmt.Sprintf("t%d", i), NodeID: fmt.Sprintf("ns=4;i=%d", i+1), DataType: core.ValueFloat64},
			Parsed: core.ParsedNodeID{Namespace: 4, IdentifierType: "i", Identifier: fmt.Sprintf("%d", i+1)},
		})
	}
	out := make(chan core.Sample, 200)
	if err := d.pollOnce(context.Background(), views, out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 120 {
		t.Fatalf("got %d samples", len(out))
	}
	sFirst := <-out
	if sFirst.Quality != core.QualityGood || sFirst.ValueNum == nil || *sFirst.ValueNum < 1 {
		t.Fatalf("first sample %#v", sFirst)
	}
	if sFirst.TagID == "" {
		t.Fatal("missing TagID")
	}

	// map error → Bad sample
	sBad := &mockSession{readFn: func(_ context.Context, req *ua.ReadRequest) (*ua.ReadResponse, error) {
		v, _ := ua.NewVariant("not-a-float")
		return &ua.ReadResponse{Results: []*ua.DataValue{{Status: ua.StatusOK, Value: v}}}, nil
	}}
	dBad := driverWithSession(sBad)
	ch := make(chan core.Sample, 1)
	if err := dBad.pollBatch(context.Background(), views[:1], ch); err != nil {
		t.Fatal(err)
	}
	s0 := <-ch
	if s0.Quality != core.QualityBad || s0.TagID != "t0" {
		t.Fatalf("%#v", s0)
	}

	sErr := &mockSession{readFn: func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
		return nil, errors.New("read fail")
	}}
	dErr := driverWithSession(sErr)
	if err := dErr.pollBatch(context.Background(), views[:1], make(chan core.Sample, 1)); err == nil {
		t.Fatal("read fail")
	}
}

func TestWriteValue_AndReadValue_Mock(t *testing.T) {
	var wrote *ua.WriteRequest
	s := &mockSession{
		writeFn: func(_ context.Context, req *ua.WriteRequest) (*ua.WriteResponse, error) {
			wrote = req
			return &ua.WriteResponse{Results: []ua.StatusCode{ua.StatusOK}}, nil
		},
		readFn: func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
			v, _ := ua.NewVariant(1.5)
			return &ua.ReadResponse{Results: []*ua.DataValue{{Status: ua.StatusOK, Value: v}}}, nil
		},
	}
	d := driverWithSession(s)
	tag := core.Tag{ID: "t", NodeID: "ns=4;i=1", DataType: core.ValueFloat64}
	if err := d.WriteValue(context.Background(), tag, 1.5); err != nil {
		t.Fatal(err)
	}
	if wrote == nil || len(wrote.NodesToWrite) != 1 {
		t.Fatalf("write req %#v", wrote)
	}
	wv := wrote.NodesToWrite[0]
	if wv.NodeID == nil || wv.NodeID.IntID() != 1 || wv.AttributeID != ua.AttributeIDValue {
		t.Fatalf("write target %#v", wv)
	}
	if wv.Value == nil || wv.Value.Value == nil {
		t.Fatalf("write value %#v", wv.Value)
	}
	if f, ok := wv.Value.Value.Value().(float64); !ok || f != 1.5 {
		t.Fatalf("write payload %#v", wv.Value.Value.Value())
	}
	sample, err := d.ReadValue(context.Background(), tag)
	if err != nil || sample.Quality != core.QualityGood || sample.TagID != "t" {
		t.Fatalf("%#v %v", sample, err)
	}
	if sample.ValueNum == nil || *sample.ValueNum != 1.5 {
		t.Fatalf("read value %#v", sample.ValueNum)
	}

	sRej := &mockSession{writeFn: func(context.Context, *ua.WriteRequest) (*ua.WriteResponse, error) {
		return &ua.WriteResponse{Results: []ua.StatusCode{ua.StatusBad}}, nil
	}}
	dRej := driverWithSession(sRej)
	err = dRej.WriteValue(context.Background(), tag, 1.0)
	var wst *WriteStatusError
	if err == nil || !errors.As(err, &wst) {
		t.Fatalf("reject: %v", err)
	}
	if wst.Status != ua.StatusBad {
		t.Fatalf("status %#v", wst.Status)
	}

	sEmpty := &mockSession{writeFn: func(context.Context, *ua.WriteRequest) (*ua.WriteResponse, error) {
		return &ua.WriteResponse{}, nil
	}}
	dEmpty := driverWithSession(sEmpty)
	if err := dEmpty.WriteValue(context.Background(), tag, 1.0); err == nil {
		t.Fatal("empty")
	}

	sFail := &mockSession{writeFn: func(context.Context, *ua.WriteRequest) (*ua.WriteResponse, error) {
		return nil, errors.New("w")
	}}
	dFail := driverWithSession(sFail)
	if err := dFail.WriteValue(context.Background(), tag, 1.0); err == nil {
		t.Fatal("transport")
	}

	dAlive := New(core.Device{ID: "x"}, nil)
	dAlive.alive.Store(true) // Connected but no client
	if err := dAlive.WriteValue(context.Background(), tag, 1.0); err == nil {
		t.Fatal("nil client")
	}
	if _, err := dAlive.ReadValue(context.Background(), tag); err == nil {
		t.Fatal("read nil client")
	}
}

func TestExpandFromRefs_ObjectWithoutKidsSkipped(t *testing.T) {
	s := &mockSession{browseFn: func(_ context.Context, req *ua.BrowseRequest) (*ua.BrowseResponse, error) {
		results := make([]*ua.BrowseResult, len(req.NodesToBrowse))
		for i := range results {
			results[i] = &ua.BrowseResult{StatusCode: ua.StatusOK}
		}
		return &ua.BrowseResponse{Results: results}, nil
	}}
	d := driverWithSession(s)
	var out []core.ExpandedTag
	refs := []*ua.ReferenceDescription{refObj(4, 99, "EmptyFolder")}
	if err := d.expandFromRefs(context.Background(), refs, "p", "", 0, 4, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("%#v", out)
	}
}

func TestBrowseRefsAt_ParseAndOK(t *testing.T) {
	s := &mockSession{browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
		return okBrowse(refVar(4, 1, "a")), nil
	}}
	d := driverWithSession(s)
	refs, err := d.browseRefsAt(context.Background(), "ns=4;i=7")
	if err != nil || len(refs) != 1 {
		t.Fatalf("%v %v", refs, err)
	}
	if refs[0].BrowseName == nil || refs[0].BrowseName.Name != "a" || refs[0].NodeID.NodeID.IntID() != 1 {
		t.Fatalf("ref %#v", refs[0])
	}
	if _, err := d.browseRefsAt(context.Background(), "bad"); err == nil {
		t.Fatal("parse")
	}
}

func TestBrowseManyReferences_NextErrors(t *testing.T) {
	d := &Driver{}
	descs := []*ua.BrowseDescription{{NodeID: ua.NewNumericNodeID(0, 1)}}
	c := &mockBrowseClient{
		browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
			return &ua.BrowseResponse{Results: []*ua.BrowseResult{{
				StatusCode:        ua.StatusOK,
				ContinuationPoint: []byte{1},
			}}}, nil
		},
		browseNextFn: func(context.Context, *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error) {
			return nil, errors.New("next")
		},
	}
	if _, err := d.browseManyReferences(context.Background(), c, descs); err == nil {
		t.Fatal("next err")
	}
	c = &mockBrowseClient{
		browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
			return &ua.BrowseResponse{Results: []*ua.BrowseResult{{
				StatusCode:        ua.StatusOK,
				ContinuationPoint: []byte{1},
			}}}, nil
		},
		browseNextFn: func(context.Context, *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error) {
			return &ua.BrowseNextResponse{}, nil
		},
	}
	got, err := d.browseManyReferences(context.Background(), c, descs)
	if err != nil || len(got[0]) != 0 {
		t.Fatalf("%#v %v", got, err)
	}
	c = &mockBrowseClient{
		browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
			return &ua.BrowseResponse{Results: []*ua.BrowseResult{{
				StatusCode:        ua.StatusOK,
				ContinuationPoint: []byte{1},
			}}}, nil
		},
		browseNextFn: func(context.Context, *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error) {
			return &ua.BrowseNextResponse{Results: []*ua.BrowseResult{{StatusCode: ua.StatusBad}}}, nil
		},
	}
	if _, err := d.browseManyReferences(context.Background(), c, descs); err == nil {
		t.Fatal("next bad")
	}
}

func TestGuessDataType_Harvesting(t *testing.T) {
	if got := GuessDataType("cell_harvesting_flag"); got != core.ValueBool {
		t.Fatalf("%q", got)
	}
}
