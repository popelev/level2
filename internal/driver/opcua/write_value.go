package opcua

import (
	"context"
	"fmt"
	"math"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
)

// WriteStatusError is returned when the OPC server rejects a Write with a non-Good StatusCode.
type WriteStatusError struct {
	Status ua.StatusCode
}

func (e *WriteStatusError) Error() string {
	if e == nil {
		return "opc write status error"
	}
	return e.Status.Error()
}

// CoerceWriteValue converts a JSON-decoded value into a Go scalar suitable for ua.NewVariant.
// Delegates to core.CoerceWriteValue (neutral package; kept here for call-site compatibility).
func CoerceWriteValue(dt core.ValueType, v any) (any, error) {
	return core.CoerceWriteValue(dt, v)
}

// ReadValue implements core.ValueReader — single-node OPC UA AttributeIDValue read (write-then-verify).
// Does not hold the client mutex across sleeps; safe to call after WriteValue without nesting locks.
func (d *Driver) ReadValue(ctx context.Context, tag core.Tag) (core.Sample, error) {
	if !d.Connected() {
		return core.Sample{}, fmt.Errorf("not connected")
	}
	views, err := PrepareTags([]core.Tag{tag})
	if err != nil {
		return core.Sample{}, err
	}
	ch := make(chan core.Sample, 1)
	if err := d.pollBatch(ctx, views, ch); err != nil {
		diag.OPCWrite(diag.LevelError, d.device.ID, tag.ID, "opc verify read failed", err.Error())
		return core.Sample{}, err
	}
	select {
	case s := <-ch:
		return s, nil
	default:
		return core.Sample{}, fmt.Errorf("empty read response")
	}
}

// WriteValue implements core.ValueWriter — single-node OPC UA AttributeIDValue write.
// On BadTypeMismatch, performs one auto-retry after coercing float64↔float32 / int64↔int32(+int16)
// using the node's OPC DataType Attribute (or a heuristic alternate width when the attribute is unavailable).
func (d *Driver) WriteValue(ctx context.Context, tag core.Tag, value any) error {
	if !d.Connected() {
		return fmt.Errorf("not connected")
	}
	parsed, err := core.ParseNodeID(tag.NodeID)
	if err != nil {
		return fmt.Errorf("tag %q: %w", tag.ID, err)
	}

	d.mu.Lock()
	c := d.client
	d.mu.Unlock()
	if c == nil {
		return fmt.Errorf("not connected")
	}

	nid, err := d.toUANodeID(ctx, parsed)
	if err != nil {
		return err
	}

	st, err := d.writeVariant(ctx, c, nid, value)
	if err != nil {
		diag.OPCWrite(diag.LevelError, d.device.ID, tag.ID, "opc write transport failed", err.Error())
		return err
	}
	if st == ua.StatusOK {
		diag.OPCWrite(diag.LevelInfo, d.device.ID, tag.ID, "opc write ok", fmt.Sprintf("node=%s value=%v", tag.NodeID, value))
		return nil
	}

	if st == ua.StatusBadTypeMismatch {
		if retryVal, ok := d.retryValueForTypeMismatch(ctx, c, nid, value); ok {
			diag.OPCWrite(diag.LevelWarn, d.device.ID, tag.ID, "opc write typemismatch retry",
				fmt.Sprintf("node=%s from=%T(%v) to=%T(%v)", tag.NodeID, value, value, retryVal, retryVal))
			st2, err2 := d.writeVariant(ctx, c, nid, retryVal)
			if err2 != nil {
				diag.OPCWrite(diag.LevelError, d.device.ID, tag.ID, "opc write transport failed", err2.Error())
				return err2
			}
			if st2 == ua.StatusOK {
				diag.OPCWrite(diag.LevelInfo, d.device.ID, tag.ID, "opc write ok after typemismatch retry",
					fmt.Sprintf("node=%s value=%v type=%T", tag.NodeID, retryVal, retryVal))
				return nil
			}
			err := &WriteStatusError{Status: st2}
			diag.OPCWrite(diag.LevelWarn, d.device.ID, tag.ID, "opc write rejected after typemismatch retry", err.Error())
			return err
		}
	}

	err = &WriteStatusError{Status: st}
	diag.OPCWrite(diag.LevelWarn, d.device.ID, tag.ID, "opc write rejected", err.Error())
	return err
}

func (d *Driver) writeVariant(ctx context.Context, c uaSession, nid *ua.NodeID, value any) (ua.StatusCode, error) {
	variant, err := ua.NewVariant(value)
	if err != nil {
		return 0, fmt.Errorf("variant: %w", err)
	}
	req := &ua.WriteRequest{
		NodesToWrite: []*ua.WriteValue{{
			NodeID:      nid,
			AttributeID: ua.AttributeIDValue,
			Value: &ua.DataValue{
				EncodingMask: ua.DataValueValue,
				Value:        variant,
			},
		}},
	}
	resp, err := c.Write(ctx, req)
	if err != nil {
		return 0, err
	}
	if resp == nil || len(resp.Results) == 0 {
		return 0, fmt.Errorf("empty write response")
	}
	return resp.Results[0], nil
}

// retryValueForTypeMismatch picks one alternate-width Go scalar for a BadTypeMismatch retry.
// Prefers OPC DataType Attribute (ns=0 built-ins); falls back to heuristic float64↔float32 / int64↔int32.
func (d *Driver) retryValueForTypeMismatch(ctx context.Context, c uaSession, nid *ua.NodeID, value any) (any, bool) {
	if typeID, ok := readDataTypeID(ctx, c, nid); ok {
		if v, ok := coerceToOPCWireType(typeID, value); ok {
			return v, true
		}
	}
	return alternateWriteWidth(value)
}

func readDataTypeID(ctx context.Context, c uaSession, nid *ua.NodeID) (uint32, bool) {
	if c == nil || nid == nil {
		return 0, false
	}
	resp, err := c.Read(ctx, &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{{
			NodeID:      nid,
			AttributeID: ua.AttributeIDDataType,
		}},
	})
	if err != nil || resp == nil || len(resp.Results) == 0 {
		return 0, false
	}
	rv := resp.Results[0]
	if rv == nil || rv.Status != ua.StatusOK || rv.Value == nil {
		return 0, false
	}
	typeNID, ok := rv.Value.Value().(*ua.NodeID)
	if !ok || typeNID == nil || typeNID.Namespace() != 0 {
		return 0, false
	}
	return typeNID.IntID(), true
}

// coerceToOPCWireType converts value to the Go scalar matching ns=0 DataType IntID.
// Returns false when the type is unsupported or the value already has that Go type
// (retry would be identical).
func coerceToOPCWireType(typeID uint32, value any) (any, bool) {
	switch typeID {
	case id.Float:
		f, ok := asFloat64Any(value)
		if !ok {
			return nil, false
		}
		if _, is := value.(float32); is {
			return nil, false
		}
		return float32(f), true
	case id.Double:
		f, ok := asFloat64Any(value)
		if !ok {
			return nil, false
		}
		if _, is := value.(float64); is {
			return nil, false
		}
		return f, true
	case id.Int16:
		n, ok := asInt64Any(value)
		if !ok || n < math.MinInt16 || n > math.MaxInt16 {
			return nil, false
		}
		if _, is := value.(int16); is {
			return nil, false
		}
		return int16(n), true
	case id.Int32:
		n, ok := asInt64Any(value)
		if !ok || n < math.MinInt32 || n > math.MaxInt32 {
			return nil, false
		}
		if _, is := value.(int32); is {
			return nil, false
		}
		return int32(n), true
	case id.Int64:
		n, ok := asInt64Any(value)
		if !ok {
			return nil, false
		}
		if _, is := value.(int64); is {
			return nil, false
		}
		return n, true
	default:
		return nil, false
	}
}

// alternateWriteWidth is the fallback when DataType Attribute is missing/unmapped:
// float64↔float32, int64↔int32.
func alternateWriteWidth(value any) (any, bool) {
	switch v := value.(type) {
	case float64:
		return float32(v), true
	case float32:
		return float64(v), true
	case int64:
		if v < math.MinInt32 || v > math.MaxInt32 {
			return nil, false
		}
		return int32(v), true
	case int32:
		return int64(v), true
	case int:
		if int64(v) < math.MinInt32 || int64(v) > math.MaxInt32 {
			return nil, false
		}
		return int32(v), true
	default:
		return nil, false
	}
}

func asFloat64Any(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case int16:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

func asInt64Any(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case int16:
		return int64(n), true
	case int:
		return int64(n), true
	case float64:
		if n != math.Trunc(n) || n < math.MinInt64 || n > math.MaxInt64 {
			return 0, false
		}
		return int64(n), true
	case float32:
		f := float64(n)
		if f != math.Trunc(f) || f < math.MinInt64 || f > math.MaxInt64 {
			return 0, false
		}
		return int64(f), true
	default:
		return 0, false
	}
}
