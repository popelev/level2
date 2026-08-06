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
	mapped, err := d.readOPCDataType(ctx, nodeID)
	if err != nil {
		mapped = ""
	}
	return resolveMappedDataType(mapped, browseHint)
}

// ApplyDataTypesFromOPC sets DataType on each tag from OPC UA (batched Attribute Read).
// Always overwrites tag.DataType (Sync from OPC) — including previously valid float64.
// GuessDataType is used when a node's DataType Read fails/unmapped, and to refine
// Siemens ByteString/wrong-Float DATE_AND_TIME or bare Time → datetime.
func ApplyDataTypesFromOPC(ctx context.Context, d *Driver, tags []core.Tag) {
	if d == nil || len(tags) == 0 {
		return
	}
	ids := make([]string, len(tags))
	for i := range tags {
		ids[i] = tags[i].NodeID
	}
	types := d.readOPCDataTypesBatch(ctx, ids, nil)
	for i := range tags {
		hint := tags[i].ID
		if hint == "" {
			hint = tags[i].NodeID
		}
		tags[i].DataType = resolveMappedDataType(types[i], hint)
	}
}

func (d *Driver) readOPCDataType(ctx context.Context, nodeID string) (core.ValueType, error) {
	types := d.readOPCDataTypesBatch(ctx, []string{nodeID}, nil)
	if len(types) == 0 || types[0] == "" {
		return "", fmt.Errorf("read datatype failed")
	}
	return types[0], nil
}

// readOPCDataTypesBatch reads AttributeIDDataType for many nodes in chunks of maxNodesPerRead.
// Empty entries mean the read failed or the OPC type is unmapped. onChunk(done, total) is
// optional and called after each chunk (done is the exclusive end index).
func (d *Driver) readOPCDataTypesBatch(ctx context.Context, nodeIDs []string, onChunk func(done, total int)) []core.ValueType {
	out := make([]core.ValueType, len(nodeIDs))
	if len(nodeIDs) == 0 {
		return out
	}
	d.mu.Lock()
	c := d.client
	d.mu.Unlock()
	if c == nil {
		if onChunk != nil {
			onChunk(len(nodeIDs), len(nodeIDs))
		}
		return out
	}

	uaIDs := make([]*ua.NodeID, len(nodeIDs))
	for i, idStr := range nodeIDs {
		parsed, err := core.ParseNodeID(idStr)
		if err != nil {
			continue
		}
		nid, err := d.toUANodeID(ctx, parsed)
		if err != nil {
			continue
		}
		uaIDs[i] = nid
	}

	total := len(nodeIDs)
	for start := 0; start < total; start += maxNodesPerRead {
		end := start + maxNodesPerRead
		if end > total {
			end = total
		}
		var nodes []*ua.ReadValueID
		var idxs []int
		for i := start; i < end; i++ {
			if uaIDs[i] == nil {
				continue
			}
			idxs = append(idxs, i)
			nodes = append(nodes, &ua.ReadValueID{
				NodeID:      uaIDs[i],
				AttributeID: ua.AttributeIDDataType,
			})
		}
		if len(nodes) > 0 {
			resp, err := c.Read(ctx, &ua.ReadRequest{NodesToRead: nodes})
			if err == nil && resp != nil {
				for j, rv := range resp.Results {
					if j >= len(idxs) || rv == nil || rv.Status != ua.StatusOK || rv.Value == nil {
						continue
					}
					typeNID, ok := rv.Value.Value().(*ua.NodeID)
					if !ok || typeNID == nil {
						continue
					}
					if dt := mapOPCDataType(typeNID); dt != "" {
						out[idxs[j]] = dt
					}
				}
			}
		}
		if onChunk != nil {
			onChunk(end, total)
		}
	}
	return out
}

func mapOPCDataType(typeNID *ua.NodeID) core.ValueType {
	if typeNID == nil {
		return ""
	}
	if typeNID.Namespace() != 0 {
		// Vendor types (e.g. Siemens DATE_AND_TIME) — caller may refine via name hint.
		return ""
	}
	switch typeNID.IntID() {
	case id.Boolean:
		return core.ValueBool
	case id.Float, id.Double:
		return core.ValueFloat64
	case id.SByte, id.Int16, id.Int32, id.Int64:
		return core.ValueInt64
	case id.Byte, id.UInt16, id.UInt32, id.UInt64:
		return core.ValueUint
	case id.String, id.LocalizedText, id.ByteString, id.XMLElement:
		// String / CharArray-as-String / LocalizedText / XMLElement.
		// ByteString is also how Siemens often exposes DATE_AND_TIME; refined by name.
		return core.ValueString
	case id.DateTime, id.UtcTime: // DateTime=i=13, UtcTime=i=294 (subtype)
		return core.ValueDateTime
	default:
		return ""
	}
}

// resolveMappedDataType combines OPC DataType Attribute mapping with browse-name hints.
// Siemens DT is frequently ByteString (→ string), vendor ns≠0 (→ empty), or mis-advertised
// as Float/Double while the Value is still a DATE_AND_TIME ByteArray — name refine wins then.
func resolveMappedDataType(mapped core.ValueType, hint string) core.ValueType {
	guess := GuessDataType(hint)
	if mapped == "" {
		return guess
	}
	// Prefer name-based Siemens refine when the Attribute type is ambiguous/wrong for plant tags.
	if guess == core.ValueDateTime && (mapped == core.ValueString || mapped == core.ValueFloat64) {
		return core.ValueDateTime
	}
	if guess == core.ValueString && mapped == core.ValueFloat64 {
		return core.ValueString
	}
	return mapped
}

// GuessDataType infers platform type from browse/signal name when OPC metadata is unavailable.
// Used as fallback after DataType Attribute Read fails, and to refine ByteString→datetime.
// Hint may be a short browse name (sUnit) or a full tag id (…_current_sunit).
func GuessDataType(browseName string) core.ValueType {
	n := strings.ToLower(browseName)
	switch {
	case looksLikeStringName(n):
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
	case looksLikeDateTimeName(n):
		return core.ValueDateTime
	case strings.HasPrefix(n, "i") || strings.Contains(n, "count"):
		return core.ValueInt64
	default:
		return core.ValueFloat64
	}
}

// looksLikeStringName detects Siemens/generic string leaves: sUnit, unit, *_sunit, sName, sText.
// Uses the last path/id segment so full tag ids (…_current_sunit) still match.
func looksLikeStringName(n string) bool {
	if strings.Contains(n, "sunit") {
		return true
	}
	base := n
	if i := strings.LastIndexAny(n, "._"); i >= 0 && i+1 < len(n) {
		base = n[i+1:]
	}
	switch base {
	case "unit", "name", "text":
		return true
	}
	// Hungarian notation on the leaf: sUnit, sName, sText.
	if strings.HasPrefix(base, "s") && (strings.Contains(base, "unit") ||
		strings.Contains(base, "name") || strings.Contains(base, "text")) {
		return true
	}
	return false
}

// looksLikeDateTimeName detects OPC DateTime / Siemens DATE_AND_TIME naming
// (e.g. LastCycleDateAndTime — "dateandtime", not "datetime"; leaf "Time" / *_time).
func looksLikeDateTimeName(n string) bool {
	if strings.Contains(n, "runtime") || strings.Contains(n, "timeout") ||
		strings.Contains(n, "lifetime") {
		return false
	}
	if strings.Contains(n, "datetime") || strings.Contains(n, "dateandtime") ||
		strings.Contains(n, "date_and_time") || strings.Contains(n, "date_time") ||
		strings.Contains(n, "timestamp") {
		return true
	}
	base := n
	if i := strings.LastIndexAny(n, "._"); i >= 0 && i+1 < len(n) {
		base = n[i+1:]
	}
	if base == "time" || base == "date" {
		return true
	}
	return strings.HasSuffix(n, "_time")
}

func browseNameHint(t core.ExpandedTag) string {
	if t.BrowsePath != "" {
		parts := strings.Split(t.BrowsePath, ".")
		return parts[len(parts)-1]
	}
	if t.ID != "" {
		return t.ID
	}
	return t.NodeID
}
