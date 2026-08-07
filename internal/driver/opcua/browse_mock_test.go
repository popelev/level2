package opcua

import (
	"context"
	"errors"
	"testing"

	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

type mockBrowseClient struct {
	browseFn     func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error)
	browseNextFn func(context.Context, *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error)
}

func (m *mockBrowseClient) Browse(ctx context.Context, req *ua.BrowseRequest) (*ua.BrowseResponse, error) {
	if m.browseFn != nil {
		return m.browseFn(ctx, req)
	}
	return nil, errors.New("browse not stubbed")
}

func (m *mockBrowseClient) BrowseNext(ctx context.Context, req *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error) {
	if m.browseNextFn != nil {
		return m.browseNextFn(ctx, req)
	}
	return nil, errors.New("browseNext not stubbed")
}

func TestBrowseAllReferences_EmptyAndStatus(t *testing.T) {
	d := &Driver{}
	desc := &ua.BrowseDescription{NodeID: ua.NewNumericNodeID(0, 85)}

	c := &mockBrowseClient{browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
		return &ua.BrowseResponse{}, nil
	}}
	refs, err := d.browseAllReferences(context.Background(), c, desc)
	if err != nil || refs != nil {
		t.Fatalf("empty results: %#v %v", refs, err)
	}

	c = &mockBrowseClient{browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
		return &ua.BrowseResponse{Results: []*ua.BrowseResult{{StatusCode: ua.StatusBad}}}, nil
	}}
	if _, err := d.browseAllReferences(context.Background(), c, desc); err == nil {
		t.Fatal("bad status")
	}

	c = &mockBrowseClient{browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
		return nil, errors.New("transport")
	}}
	if _, err := d.browseAllReferences(context.Background(), c, desc); err == nil {
		t.Fatal("transport")
	}
}

func TestBrowseAllReferences_Continuation(t *testing.T) {
	d := &Driver{}
	desc := &ua.BrowseDescription{NodeID: ua.NewNumericNodeID(4, 1)}
	ref1 := &ua.ReferenceDescription{
		NodeID:     &ua.ExpandedNodeID{NodeID: ua.NewNumericNodeID(4, 2)},
		BrowseName: &ua.QualifiedName{Name: "A"},
		NodeClass:  ua.NodeClassVariable,
	}
	ref2 := &ua.ReferenceDescription{
		NodeID:     &ua.ExpandedNodeID{NodeID: ua.NewNumericNodeID(4, 3)},
		BrowseName: &ua.QualifiedName{Name: "B"},
		NodeClass:  ua.NodeClassObject,
	}
	calls := 0
	c := &mockBrowseClient{
		browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
			return &ua.BrowseResponse{Results: []*ua.BrowseResult{{
				StatusCode:         ua.StatusOK,
				References:         []*ua.ReferenceDescription{ref1},
				ContinuationPoint:  []byte{1, 2},
			}}}, nil
		},
		browseNextFn: func(context.Context, *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error) {
			calls++
			return &ua.BrowseNextResponse{Results: []*ua.BrowseResult{{
				StatusCode:        ua.StatusOK,
				References:        []*ua.ReferenceDescription{ref2},
				ContinuationPoint: nil,
			}}}, nil
		},
	}
	refs, err := d.browseAllReferences(context.Background(), c, desc)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 || calls != 1 {
		t.Fatalf("refs=%d calls=%d", len(refs), calls)
	}
}

func TestBrowseAllReferences_BrowseNextErrors(t *testing.T) {
	d := &Driver{}
	desc := &ua.BrowseDescription{NodeID: ua.NewNumericNodeID(4, 1)}
	base := func() *ua.BrowseResponse {
		return &ua.BrowseResponse{Results: []*ua.BrowseResult{{
			StatusCode:        ua.StatusOK,
			ContinuationPoint: []byte{9},
		}}}
	}

	c := &mockBrowseClient{
		browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) { return base(), nil },
		browseNextFn: func(context.Context, *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error) {
			return nil, errors.New("next fail")
		},
	}
	if _, err := d.browseAllReferences(context.Background(), c, desc); err == nil {
		t.Fatal("next transport")
	}

	c = &mockBrowseClient{
		browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) { return base(), nil },
		browseNextFn: func(context.Context, *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error) {
			return &ua.BrowseNextResponse{}, nil
		},
	}
	refs, err := d.browseAllReferences(context.Background(), c, desc)
	if err != nil || len(refs) != 0 {
		t.Fatalf("empty next: %#v %v", refs, err)
	}

	c = &mockBrowseClient{
		browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) { return base(), nil },
		browseNextFn: func(context.Context, *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error) {
			return &ua.BrowseNextResponse{Results: []*ua.BrowseResult{{StatusCode: ua.StatusBad}}}, nil
		},
	}
	if _, err := d.browseAllReferences(context.Background(), c, desc); err == nil {
		t.Fatal("next bad status")
	}
}

func TestBrowseManyReferences_Edges(t *testing.T) {
	d := &Driver{}
	out, err := d.browseManyReferences(context.Background(), &mockBrowseClient{}, nil)
	if err != nil || out != nil {
		t.Fatalf("empty descs %#v %v", out, err)
	}

	descs := []*ua.BrowseDescription{{NodeID: ua.NewNumericNodeID(0, 1)}}
	c := &mockBrowseClient{browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
		return nil, errors.New("fail")
	}}
	if _, err := d.browseManyReferences(context.Background(), c, descs); err == nil {
		t.Fatal("transport")
	}

	c = &mockBrowseClient{browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
		return &ua.BrowseResponse{}, nil
	}}
	if _, err := d.browseManyReferences(context.Background(), c, descs); err == nil {
		t.Fatal("len mismatch")
	}

	c = &mockBrowseClient{browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
		return &ua.BrowseResponse{Results: []*ua.BrowseResult{nil}}, nil
	}}
	got, err := d.browseManyReferences(context.Background(), c, descs)
	if err != nil || len(got) != 1 || got[0] != nil {
		t.Fatalf("nil result %#v %v", got, err)
	}

	c = &mockBrowseClient{browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
		return &ua.BrowseResponse{Results: []*ua.BrowseResult{{StatusCode: ua.StatusBad}}}, nil
	}}
	if _, err := d.browseManyReferences(context.Background(), c, descs); err == nil {
		t.Fatal("bad status")
	}

	ref := &ua.ReferenceDescription{BrowseName: &ua.QualifiedName{Name: "X"}}
	nextCalls := 0
	c = &mockBrowseClient{
		browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
			return &ua.BrowseResponse{Results: []*ua.BrowseResult{{
				StatusCode:        ua.StatusOK,
				References:        []*ua.ReferenceDescription{ref},
				ContinuationPoint: []byte{1},
			}}}, nil
		},
		browseNextFn: func(context.Context, *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error) {
			nextCalls++
			return &ua.BrowseNextResponse{Results: []*ua.BrowseResult{{
				StatusCode: ua.StatusOK,
				References: []*ua.ReferenceDescription{ref},
			}}}, nil
		},
	}
	got, err = d.browseManyReferences(context.Background(), c, descs)
	if err != nil || len(got) != 1 || len(got[0]) != 2 || nextCalls != 1 {
		t.Fatalf("cont %#v calls=%d err=%v", got, nextCalls, err)
	}
}

func TestIsScalarVariableLeaf(t *testing.T) {
	d := &Driver{}
	obj := &ua.ReferenceDescription{
		NodeClass: ua.NodeClassObject,
		NodeID:    &ua.ExpandedNodeID{NodeID: ua.NewNumericNodeID(4, 1)},
	}
	if d.isScalarVariableLeaf(context.Background(), &mockBrowseClient{}, obj) {
		t.Fatal("object not leaf")
	}

	leaf := &ua.ReferenceDescription{
		NodeClass: ua.NodeClassVariable,
		NodeID:    &ua.ExpandedNodeID{NodeID: ua.NewNumericNodeID(4, 2)},
	}
	c := &mockBrowseClient{browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
		return &ua.BrowseResponse{Results: []*ua.BrowseResult{{StatusCode: ua.StatusOK}}}, nil
	}}
	if !d.isScalarVariableLeaf(context.Background(), c, leaf) {
		t.Fatal("empty kids → leaf")
	}

	c = &mockBrowseClient{browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
		return &ua.BrowseResponse{Results: []*ua.BrowseResult{{
			StatusCode: ua.StatusOK,
			References: []*ua.ReferenceDescription{{BrowseName: &ua.QualifiedName{Name: "f"}}},
		}}}, nil
	}}
	if d.isScalarVariableLeaf(context.Background(), c, leaf) {
		t.Fatal("has kids → not leaf")
	}

	c = &mockBrowseClient{browseFn: func(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error) {
		return nil, errors.New("err")
	}}
	if !d.isScalarVariableLeaf(context.Background(), c, leaf) {
		t.Fatal("browse err → treat as leaf")
	}
}

func TestFormatNodeID_GUID(t *testing.T) {
	g := ua.NewGUIDNodeID(2, "00112233-4455-6677-8899-aabbccddeeff")
	got := formatNodeID(g)
	if got == "" || got[:4] != "ns=2" {
		t.Fatalf("%q", got)
	}
}

func TestBrowseOffline_NotConnected(t *testing.T) {
	d := New(core.Device{ID: "d"}, nil)
	ctx := context.Background()
	if _, err := d.BrowseChildren(ctx, "ns=4;i=1"); err == nil {
		t.Fatal("BrowseChildren")
	}
	if _, _, err := d.ProbeNode(ctx, "ns=4;i=1"); err == nil {
		t.Fatal("ProbeNode")
	}
	if _, err := d.ExpandStructure(ctx, "ns=4;i=1", "p", 2); err == nil {
		t.Fatal("ExpandStructure")
	}
	if _, err := d.browseRefsAt(ctx, "ns=4;i=1"); err == nil {
		t.Fatal("browseRefsAt")
	}
	if _, err := d.browseRefsMulti(ctx, []*ua.NodeID{ua.NewNumericNodeID(4, 1)}); err == nil {
		t.Fatal("browseRefsMulti")
	}
}
