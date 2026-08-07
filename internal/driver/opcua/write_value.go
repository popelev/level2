package opcua

import (
	"context"
	"fmt"

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
func (d *Driver) WriteValue(ctx context.Context, tag core.Tag, value any) error {
	if !d.Connected() {
		return fmt.Errorf("not connected")
	}
	parsed, err := core.ParseNodeID(tag.NodeID)
	if err != nil {
		return fmt.Errorf("tag %q: %w", tag.ID, err)
	}

	variant, err := ua.NewVariant(value)
	if err != nil {
		return fmt.Errorf("variant: %w", err)
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
		diag.OPCWrite(diag.LevelError, d.device.ID, tag.ID, "opc write transport failed", err.Error())
		return err
	}
	if resp == nil || len(resp.Results) == 0 {
		return fmt.Errorf("empty write response")
	}
	st := resp.Results[0]
	if st != ua.StatusOK {
		err := &WriteStatusError{Status: st}
		diag.OPCWrite(diag.LevelWarn, d.device.ID, tag.ID, "opc write rejected", err.Error())
		return err
	}
	diag.OPCWrite(diag.LevelInfo, d.device.ID, tag.ID, "opc write ok", fmt.Sprintf("node=%s value=%v", tag.NodeID, value))
	return nil
}
