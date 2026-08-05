package opcua

import (
	"context"
	"fmt"
	"strings"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

// BrowseChildren lists direct children of parent NodeId (e.g. ns=4;i=4207).
func (d *Driver) BrowseChildren(ctx context.Context, parentNodeID string) ([]core.BrowseNode, error) {
	d.mu.Lock()
	c := d.client
	d.mu.Unlock()
	if c == nil {
		return nil, fmt.Errorf("not connected")
	}
	parsed, err := core.ParseNodeID(parentNodeID)
	if err != nil {
		return nil, err
	}
	nid, err := d.toUANodeID(ctx, parsed)
	if err != nil {
		return nil, err
	}
	req := &ua.BrowseRequest{
		NodesToBrowse: []*ua.BrowseDescription{{
			NodeID:          nid,
			BrowseDirection: ua.BrowseDirectionForward,
			ReferenceTypeID: ua.NewNumericNodeID(0, id.HierarchicalReferences),
			IncludeSubtypes: true,
			NodeClassMask:   uint32(ua.NodeClassObject | ua.NodeClassVariable | ua.NodeClassObjectType | ua.NodeClassVariableType),
			ResultMask:      uint32(ua.BrowseResultMaskAll),
		}},
	}
	resp, err := c.Browse(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(resp.Results) == 0 || resp.Results[0] == nil {
		return nil, nil
	}
	out := make([]core.BrowseNode, 0, len(resp.Results[0].References))
	for _, ref := range resp.Results[0].References {
		if ref == nil || ref.NodeID == nil || ref.NodeID.NodeID == nil {
			continue
		}
		bn := core.BrowseNode{
			NodeID:      formatNodeID(ref.NodeID.NodeID),
			BrowseName:  ref.BrowseName.Name,
			DisplayName: ref.DisplayName.Text,
			NodeClass:   fmt.Sprintf("%v", ref.NodeClass),
			IsLeaf:      ref.NodeClass == ua.NodeClassVariable,
		}
		out = append(out, bn)
	}
	return out, nil
}

// ProbeNode checks NodeClass via OPC UA Read (exists / is Variable).
func (d *Driver) ProbeNode(ctx context.Context, nodeID string) (exists bool, isLeaf bool, err error) {
	d.mu.Lock()
	c := d.client
	d.mu.Unlock()
	if c == nil {
		return false, false, fmt.Errorf("not connected")
	}
	parsed, err := core.ParseNodeID(nodeID)
	if err != nil {
		return false, false, err
	}
	nid, err := d.toUANodeID(ctx, parsed)
	if err != nil {
		return false, false, err
	}
	req := &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{{
			NodeID:      nid,
			AttributeID: ua.AttributeIDNodeClass,
		}},
	}
	resp, err := c.Read(ctx, req)
	if err != nil {
		return false, false, err
	}
	if len(resp.Results) == 0 || resp.Results[0] == nil {
		return false, false, nil
	}
	r := resp.Results[0]
	if r.Status != ua.StatusOK {
		return false, false, nil
	}
	nc, ok := r.Value.Value().(ua.NodeClass)
	if !ok {
		// some stacks return int32
		if v, ok2 := r.Value.Value().(int32); ok2 {
			nc = ua.NodeClass(v)
		} else {
			return true, false, nil
		}
	}
	return true, nc == ua.NodeClassVariable, nil
}

// ExpandStructure walks hierarchical children and returns leaf Variable tags.
func (d *Driver) ExpandStructure(ctx context.Context, parentNodeID, parentTagID string, maxDepth int) ([]core.ExpandedTag, error) {
	if maxDepth <= 0 {
		maxDepth = 8
	}
	if parentTagID == "" {
		parentTagID = "udt"
	}
	var out []core.ExpandedTag
	err := d.expandWalk(ctx, parentNodeID, parentTagID, "", 0, maxDepth, &out)
	return out, err
}

func (d *Driver) expandWalk(ctx context.Context, nodeID, tagPrefix, path string, depth, maxDepth int, out *[]core.ExpandedTag) error {
	if depth > maxDepth {
		return nil
	}
	children, err := d.BrowseChildren(ctx, nodeID)
	if err != nil {
		return err
	}
	for _, ch := range children {
		childPath := ch.BrowseName
		if path != "" {
			childPath = path + "." + ch.BrowseName
		}
		tagID := sanitizeTagID(tagPrefix + "_" + childPath)
		if ch.IsLeaf {
			*out = append(*out, core.ExpandedTag{
				ID:         tagID,
				NodeID:     ch.NodeID,
				BrowsePath: childPath,
				DataType:   guessDataType(ch.BrowseName),
			})
			continue
		}
		if err := d.expandWalk(ctx, ch.NodeID, tagPrefix, childPath, depth+1, maxDepth, out); err != nil {
			return err
		}
	}
	return nil
}

func formatNodeID(n *ua.NodeID) string {
	switch n.Type() {
	case ua.NodeIDTypeNumeric:
		return fmt.Sprintf("ns=%d;i=%d", n.Namespace(), n.IntID())
	case ua.NodeIDTypeString:
		return fmt.Sprintf("ns=%d;s=%s", n.Namespace(), n.StringID())
	default:
		return n.String()
	}
}

func sanitizeTagID(s string) string {
	s = strings.ToLower(s)
	repl := strings.NewReplacer(" ", "_", ".", "_", "-", "_", "/", "_")
	return repl.Replace(s)
}

func guessDataType(browseName string) core.ValueType {
	n := strings.ToLower(browseName)
	switch {
	case strings.HasPrefix(n, "s") && (strings.Contains(n, "unit") || strings.Contains(n, "name") || strings.Contains(n, "text")):
		return core.ValueString
	case strings.HasPrefix(n, "b") || strings.Contains(n, "bool") || strings.HasPrefix(n, "enable"):
		return core.ValueBool
	case strings.HasPrefix(n, "i") || strings.Contains(n, "count") || strings.Contains(n, "int"):
		return core.ValueInt64
	default:
		return core.ValueFloat64
	}
}

// ExpandFromTree is a pure helper used by unit tests / simulator without live OPC.
func ExpandFromTree(parentTagID string, nodes []core.BrowseNode) []core.ExpandedTag {
	out := make([]core.ExpandedTag, 0, len(nodes))
	for _, ch := range nodes {
		if !ch.IsLeaf {
			continue
		}
		out = append(out, core.ExpandedTag{
			ID:         sanitizeTagID(parentTagID + "_" + ch.BrowseName),
			NodeID:     ch.NodeID,
			BrowsePath: ch.BrowseName,
			DataType:   guessDataType(ch.BrowseName),
		})
	}
	return out
}
