package simbrowser

import (
	"context"
	"fmt"
	"strings"

	"github.com/popelev/level2/internal/core"
)

// Browser is an in-memory OPC tree for PLC-off UI/API work (N5/N7).
type Browser struct {
	children map[string][]core.BrowseNode
}

func NewDemo() *Browser {
	b := &Browser{children: map[string][]core.BrowseNode{}}
	objects := "ns=0;i=85"
	b.children[objects] = []core.BrowseNode{
		{NodeID: "ns=4;i=1000", BrowseName: "ServerInterfaces", DisplayName: "ServerInterfaces", NodeClass: "Object", IsLeaf: false},
	}
	b.children["ns=4;i=1000"] = []core.BrowseNode{
		{NodeID: "ns=4;i=4207", BrowseName: "OPC_MeasurePoint", DisplayName: "OPC_MeasurePoint", NodeClass: "Object", IsLeaf: false},
	}
	b.children["ns=4;i=4207"] = []core.BrowseNode{
		{NodeID: "ns=4;i=4208", BrowseName: "rValueOut", DisplayName: "rValueOut", NodeClass: "Variable", IsLeaf: true},
		{NodeID: "ns=4;i=4209", BrowseName: "sUnit", DisplayName: "sUnit", NodeClass: "Variable", IsLeaf: true},
		{NodeID: "ns=4;i=4210", BrowseName: "bValid", DisplayName: "bValid", NodeClass: "Variable", IsLeaf: true},
	}
	return b
}

func (b *Browser) BrowseChildren(_ context.Context, parentNodeID string) ([]core.BrowseNode, error) {
	nodes, ok := b.children[parentNodeID]
	if !ok {
		return nil, fmt.Errorf("unknown node %s", parentNodeID)
	}
	out := make([]core.BrowseNode, len(nodes))
	copy(out, nodes)
	return out, nil
}

func (b *Browser) ExpandStructure(ctx context.Context, parentNodeID, parentTagID string, maxDepth int) ([]core.ExpandedTag, error) {
	if maxDepth <= 0 {
		maxDepth = 8
	}
	if parentTagID == "" {
		parentTagID = "udt"
	}
	var out []core.ExpandedTag
	if err := b.walk(ctx, parentNodeID, parentTagID, "", 0, maxDepth, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (b *Browser) walk(ctx context.Context, nodeID, tagPrefix, path string, depth, maxDepth int, out *[]core.ExpandedTag) error {
	if depth > maxDepth {
		return nil
	}
	children, err := b.BrowseChildren(ctx, nodeID)
	if err != nil {
		return err
	}
	for _, ch := range children {
		childPath := ch.BrowseName
		if path != "" {
			childPath = path + "." + ch.BrowseName
		}
		if ch.IsLeaf {
			*out = append(*out, core.ExpandedTag{
				ID:         sanitize(tagPrefix + "_" + childPath),
				NodeID:     ch.NodeID,
				BrowsePath: childPath,
				DataType:   guess(ch.BrowseName),
			})
			continue
		}
		if err := b.walk(ctx, ch.NodeID, tagPrefix, childPath, depth+1, maxDepth, out); err != nil {
			return err
		}
	}
	return nil
}

func sanitize(s string) string {
	s = strings.ToLower(s)
	return strings.NewReplacer(" ", "_", ".", "_", "-", "_", "/", "_").Replace(s)
}

func guess(browseName string) core.ValueType {
	n := strings.ToLower(browseName)
	switch {
	case strings.HasPrefix(n, "s") && (strings.Contains(n, "unit") || strings.Contains(n, "name") || strings.Contains(n, "text")):
		return core.ValueString
	case strings.HasPrefix(n, "b") || strings.Contains(n, "bool"):
		return core.ValueBool
	case strings.HasPrefix(n, "i") || strings.Contains(n, "count"):
		return core.ValueInt64
	default:
		return core.ValueFloat64
	}
}
