package opcua

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

func TestAlternateWriteWidth(t *testing.T) {
	cases := []struct {
		in   any
		want any
		ok   bool
	}{
		{float64(1.5), float32(1.5), true},
		{float32(2.25), float64(2.25), true},
		{int64(42), int32(42), true},
		{int32(7), int64(7), true},
		{int(9), int32(9), true},
		{int64(math.MaxInt32 + 1), nil, false},
		{true, nil, false},
		{"x", nil, false},
	}
	for _, tc := range cases {
		got, ok := alternateWriteWidth(tc.in)
		if ok != tc.ok {
			t.Fatalf("%#v: ok=%v want %v", tc.in, ok, tc.ok)
		}
		if tc.ok && got != tc.want {
			t.Fatalf("%#v: got %#v want %#v", tc.in, got, tc.want)
		}
	}
}

func TestCoerceToOPCWireType(t *testing.T) {
	cases := []struct {
		typeID uint32
		in     any
		want   any
		ok     bool
	}{
		{id.Float, float64(1.5), float32(1.5), true},
		{id.Float, float32(1.5), nil, false}, // already float32
		{id.Double, float32(2), float64(2), true},
		{id.Double, float64(2), nil, false},
		{id.Int16, int64(100), int16(100), true},
		{id.Int16, int64(math.MaxInt16 + 1), nil, false},
		{id.Int32, int64(1000), int32(1000), true},
		{id.Int32, int32(1), nil, false},
		{id.Int64, int32(5), int64(5), true},
		{id.Int64, int64(5), nil, false},
		{id.Boolean, true, nil, false},
		{id.Float, true, nil, false},
	}
	for _, tc := range cases {
		got, ok := coerceToOPCWireType(tc.typeID, tc.in)
		if ok != tc.ok {
			t.Fatalf("type=%d in=%#v: ok=%v want %v", tc.typeID, tc.in, ok, tc.ok)
		}
		if tc.ok && got != tc.want {
			t.Fatalf("type=%d: got %#v want %#v", tc.typeID, got, tc.want)
		}
	}
}

func TestWriteValue_TypeMismatchRetry_Float32ViaDataType(t *testing.T) {
	var writes []any
	s := &mockSession{
		writeFn: func(_ context.Context, req *ua.WriteRequest) (*ua.WriteResponse, error) {
			v := req.NodesToWrite[0].Value.Value.Value()
			writes = append(writes, v)
			if _, ok := v.(float64); ok {
				return &ua.WriteResponse{Results: []ua.StatusCode{ua.StatusBadTypeMismatch}}, nil
			}
			return &ua.WriteResponse{Results: []ua.StatusCode{ua.StatusOK}}, nil
		},
		readFn: func(_ context.Context, req *ua.ReadRequest) (*ua.ReadResponse, error) {
			if len(req.NodesToRead) == 0 || req.NodesToRead[0].AttributeID != ua.AttributeIDDataType {
				return nil, errors.New("unexpected read")
			}
			typeNID := ua.NewNumericNodeID(0, id.Float)
			vv, err := ua.NewVariant(typeNID)
			if err != nil {
				return nil, err
			}
			return &ua.ReadResponse{Results: []*ua.DataValue{{Status: ua.StatusOK, Value: vv}}}, nil
		},
	}
	d := driverWithSession(s)
	tag := core.Tag{ID: "sp", NodeID: "ns=4;i=1", DataType: core.ValueFloat64}
	if err := d.WriteValue(context.Background(), tag, float64(42.5)); err != nil {
		t.Fatal(err)
	}
	if len(writes) != 2 {
		t.Fatalf("writes=%d %#v", len(writes), writes)
	}
	if _, ok := writes[0].(float64); !ok {
		t.Fatalf("first write %#v", writes[0])
	}
	if f32, ok := writes[1].(float32); !ok || f32 != float32(42.5) {
		t.Fatalf("retry write %#v", writes[1])
	}
}

func TestWriteValue_TypeMismatchRetry_Int16ViaDataType(t *testing.T) {
	var writes []any
	s := &mockSession{
		writeFn: func(_ context.Context, req *ua.WriteRequest) (*ua.WriteResponse, error) {
			v := req.NodesToWrite[0].Value.Value.Value()
			writes = append(writes, v)
			if _, ok := v.(int64); ok {
				return &ua.WriteResponse{Results: []ua.StatusCode{ua.StatusBadTypeMismatch}}, nil
			}
			return &ua.WriteResponse{Results: []ua.StatusCode{ua.StatusOK}}, nil
		},
		readFn: func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
			vv, _ := ua.NewVariant(ua.NewNumericNodeID(0, id.Int16))
			return &ua.ReadResponse{Results: []*ua.DataValue{{Status: ua.StatusOK, Value: vv}}}, nil
		},
	}
	d := driverWithSession(s)
	tag := core.Tag{ID: "i", NodeID: "ns=4;i=2", DataType: core.ValueInt64}
	if err := d.WriteValue(context.Background(), tag, int64(12)); err != nil {
		t.Fatal(err)
	}
	if len(writes) != 2 {
		t.Fatalf("writes=%d", len(writes))
	}
	if got, ok := writes[1].(int16); !ok || got != 12 {
		t.Fatalf("retry %#v", writes[1])
	}
}

func TestWriteValue_TypeMismatchRetry_HeuristicNoDataType(t *testing.T) {
	var writes []any
	s := &mockSession{
		writeFn: func(_ context.Context, req *ua.WriteRequest) (*ua.WriteResponse, error) {
			v := req.NodesToWrite[0].Value.Value.Value()
			writes = append(writes, v)
			if _, ok := v.(float64); ok {
				return &ua.WriteResponse{Results: []ua.StatusCode{ua.StatusBadTypeMismatch}}, nil
			}
			return &ua.WriteResponse{Results: []ua.StatusCode{ua.StatusOK}}, nil
		},
		readFn: func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
			return nil, errors.New("datatype unavailable")
		},
	}
	d := driverWithSession(s)
	tag := core.Tag{ID: "sp", NodeID: "ns=4;i=3", DataType: core.ValueFloat64}
	if err := d.WriteValue(context.Background(), tag, float64(3)); err != nil {
		t.Fatal(err)
	}
	if len(writes) != 2 {
		t.Fatalf("writes=%d", len(writes))
	}
	if _, ok := writes[1].(float32); !ok {
		t.Fatalf("heuristic retry %#v", writes[1])
	}
}

func TestWriteValue_TypeMismatch_NoRetryPossible(t *testing.T) {
	s := &mockSession{
		writeFn: func(context.Context, *ua.WriteRequest) (*ua.WriteResponse, error) {
			return &ua.WriteResponse{Results: []ua.StatusCode{ua.StatusBadTypeMismatch}}, nil
		},
		readFn: func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
			vv, _ := ua.NewVariant(ua.NewNumericNodeID(0, id.Boolean))
			return &ua.ReadResponse{Results: []*ua.DataValue{{Status: ua.StatusOK, Value: vv}}}, nil
		},
	}
	d := driverWithSession(s)
	err := d.WriteValue(context.Background(), core.Tag{ID: "b", NodeID: "ns=4;i=4"}, true)
	var wst *WriteStatusError
	if err == nil || !errors.As(err, &wst) || wst.Status != ua.StatusBadTypeMismatch {
		t.Fatalf("got %v", err)
	}
}

func TestWriteValue_TypeMismatchRetry_StillRejected(t *testing.T) {
	calls := 0
	s := &mockSession{
		writeFn: func(context.Context, *ua.WriteRequest) (*ua.WriteResponse, error) {
			calls++
			return &ua.WriteResponse{Results: []ua.StatusCode{ua.StatusBadTypeMismatch}}, nil
		},
		readFn: func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
			vv, _ := ua.NewVariant(ua.NewNumericNodeID(0, id.Float))
			return &ua.ReadResponse{Results: []*ua.DataValue{{Status: ua.StatusOK, Value: vv}}}, nil
		},
	}
	d := driverWithSession(s)
	err := d.WriteValue(context.Background(), core.Tag{ID: "sp", NodeID: "ns=4;i=5"}, float64(1))
	var wst *WriteStatusError
	if err == nil || !errors.As(err, &wst) || wst.Status != ua.StatusBadTypeMismatch {
		t.Fatalf("got %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestWriteValue_NonTypeMismatch_NoRetry(t *testing.T) {
	calls := 0
	s := &mockSession{
		writeFn: func(context.Context, *ua.WriteRequest) (*ua.WriteResponse, error) {
			calls++
			return &ua.WriteResponse{Results: []ua.StatusCode{ua.StatusBadUserAccessDenied}}, nil
		},
	}
	d := driverWithSession(s)
	err := d.WriteValue(context.Background(), core.Tag{ID: "sp", NodeID: "ns=4;i=6"}, float64(1))
	var wst *WriteStatusError
	if err == nil || !errors.As(err, &wst) {
		t.Fatalf("got %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestReadDataTypeID(t *testing.T) {
	if _, ok := readDataTypeID(context.Background(), nil, nil); ok {
		t.Fatal("nil session")
	}
	s := &mockSession{readFn: func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
		vv, _ := ua.NewVariant(ua.NewNumericNodeID(0, id.Int32))
		return &ua.ReadResponse{Results: []*ua.DataValue{{Status: ua.StatusOK, Value: vv}}}, nil
	}}
	got, ok := readDataTypeID(context.Background(), s, ua.NewNumericNodeID(4, 1))
	if !ok || got != id.Int32 {
		t.Fatalf("got %d ok=%v", got, ok)
	}
}
