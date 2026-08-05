package simbrowser

import (
	"context"
	"fmt"
	"strings"

	"github.com/popelev/level2/internal/core"
)

// Browser is an in-memory OPC tree shaped like a Siemens address space (PLC-off).
type Browser struct {
	children map[string][]core.BrowseNode
}

func NewDemo() *Browser {
	b := &Browser{children: map[string][]core.BrowseNode{}}
	root := "ns=0;i=84"
	objects := "ns=0;i=85"
	server := "ns=0;i=2253"
	serverIf := "ns=4;i=1000"
	deviceSet := "ns=2;i=5001"
	plc := "ns=2;i=5002"
	th1 := "ns=4;i=2000"
	th2 := "ns=4;i=2100"
	measure := "ns=4;i=4207"

	b.children[root] = []core.BrowseNode{
		folder(objects, "Objects"),
		folder(server, "Server"),
	}
	b.children[objects] = []core.BrowseNode{
		folder(deviceSet, "DeviceSet"),
		folder(serverIf, "ServerInterfaces"),
		folder(plc, "L01-KE01"),
		folder(server, "Server"),
	}
	b.children[deviceSet] = []core.BrowseNode{
		folder(plc, "L01-KE01"),
	}
	b.children[plc] = []core.BrowseNode{
		folder(th1, "Tankhouse_Data_1"),
		folder(th2, "Tankhouse_Data_2"),
	}
	b.children[serverIf] = []core.BrowseNode{
		folder(th1, "Tankhouse_Data_1"),
		folder(th2, "Tankhouse_Data_2"),
	}
	b.children[th1] = []core.BrowseNode{
		folder(measure, "OPC_MeasurePoint"),
		leaf("ns=4;i=4208", "rValueOut"),
	}
	b.children[th2] = []core.BrowseNode{
		folder(measure, "OPC_MeasurePoint"),
		leaf("ns=4;i=2880", "E2_ECE_300_CL_001"),
		leaf("ns=4;i=4431", "APM_AUTO"),
	}
	b.children[measure] = []core.BrowseNode{
		leaf("ns=4;i=4208", "rValueOut"),
		leaf("ns=4;i=4209", "sUnit"),
		leaf("ns=4;i=4210", "bValid"),
	}
	b.children[server] = []core.BrowseNode{
		leaf("ns=0;i=2256", "ServerStatus"),
	}
	return b
}

func folder(id, name string) core.BrowseNode {
	return core.BrowseNode{
		NodeID: id, BrowseName: name, DisplayName: name,
		NodeClass: "Object", IsLeaf: false,
	}
}

func leaf(id, name string) core.BrowseNode {
	return core.BrowseNode{
		NodeID: id, BrowseName: name, DisplayName: name,
		NodeClass: "Variable", IsLeaf: true,
	}
}

func (b *Browser) BrowseChildren(_ context.Context, parentNodeID string) ([]core.BrowseNode, error) {
	if parentNodeID == "" {
		parentNodeID = "ns=0;i=84"
	}
	nodes, ok := b.children[parentNodeID]
	if !ok {
		return nil, fmt.Errorf("unknown node %s", parentNodeID)
	}
	out := make([]core.BrowseNode, len(nodes))
	copy(out, nodes)
	return out, nil
}

// ProbeNode reports whether nodeID exists in the demo tree.
func (b *Browser) ProbeNode(_ context.Context, nodeID string) (exists bool, isLeaf bool, err error) {
	if nodeID == "" {
		return false, false, fmt.Errorf("empty node id")
	}
	if _, ok := b.children[nodeID]; ok {
		return true, false, nil
	}
	for _, kids := range b.children {
		for _, n := range kids {
			if n.NodeID == nodeID {
				return true, n.IsLeaf, nil
			}
		}
	}
	return false, false, nil
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
	case strings.HasPrefix(n, "b") || strings.Contains(n, "bool") || strings.Contains(n, "auto") || strings.HasSuffix(n, "_run"):
		return core.ValueBool
	case strings.HasPrefix(n, "i") || strings.Contains(n, "count"):
		return core.ValueInt64
	default:
		return core.ValueFloat64
	}
}
