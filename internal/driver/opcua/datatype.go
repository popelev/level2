package opcua

import (
	"context"
	"fmt"
	"strings"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

// ResolveTagDataType reads OPC UA DataType for nodeID; falls back to browse-name heuristics.
func (d *Driver) ResolveTagDataType(ctx context.Context, nodeID, browseHint string) core.ValueType {
	if dt, err := d.readOPCDataType(ctx, nodeID); err == nil && dt != "" {
		return dt
	}
	return GuessDataType(browseHint)
}

func (d *Driver) readOPCDataType(ctx context.Context, nodeID string) (core.ValueType, error) {
	d.mu.Lock()
	c := d.client
	d.mu.Unlock()
	if c == nil {
		return "", fmt.Errorf("not connected")
	}
	parsed, err := core.ParseNodeID(nodeID)
	if err != nil {
		return "", err
	}
	nid, err := d.toUANodeID(ctx, parsed)
	if err != nil {
		return "", err
	}
	req := &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{
			{NodeID: nid, AttributeID: ua.AttributeIDDataType},
		},
	}
	resp, err := c.Read(ctx, req)
	if err != nil {
		return "", err
	}
	if len(resp.Results) == 0 || resp.Results[0].Status != ua.StatusOK {
		return "", fmt.Errorf("read datatype status")
	}
	typeNID, ok := resp.Results[0].Value.Value().(*ua.NodeID)
	if !ok || typeNID == nil {
		return "", fmt.Errorf("datatype not node id")
	}
	return mapOPCDataType(typeNID), nil
}

func mapOPCDataType(typeNID *ua.NodeID) core.ValueType {
	if typeNID == nil {
		return ""
	}
	if typeNID.Namespace() != 0 {
		return ""
	}
	switch typeNID.IntID() {
	case id.Boolean:
		return core.ValueBool
	case id.Float, id.Double:
		return core.ValueFloat64
	case id.SByte, id.Byte, id.Int16, id.UInt16, id.Int32, id.UInt32, id.Int64, id.UInt64:
		return core.ValueInt64
	case id.String, id.LocalizedText, id.ByteString:
		return core.ValueString
	default:
		return ""
	}
}

// GuessDataType infers platform type from browse/signal name when OPC metadata is unavailable.
func GuessDataType(browseName string) core.ValueType {
	n := strings.ToLower(browseName)
	switch {
	case strings.HasPrefix(n, "s") && (strings.Contains(n, "unit") || strings.Contains(n, "name") || strings.Contains(n, "text")):
		return core.ValueString
	case strings.HasPrefix(n, "b") || strings.Contains(n, "bool") || strings.HasPrefix(n, "enable"):
		return core.ValueBool
	case strings.HasSuffix(n, "_maintenance") || strings.HasSuffix(n, "_operation"):
		return core.ValueBool
	case strings.Contains(n, "harvesting"):
		return core.ValueBool
	case strings.Contains(n, "_mode_") && !strings.Contains(n, "rvalue"):
		return core.ValueBool
	case strings.HasSuffix(n, "_auto") || strings.HasSuffix(n, "_run") || strings.HasSuffix(n, "_active"):
		return core.ValueBool
	case strings.HasPrefix(n, "i") || strings.Contains(n, "count") || strings.Contains(n, "int"):
		return core.ValueInt64
	default:
		return core.ValueFloat64
	}
}
