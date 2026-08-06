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
	desc := &ua.BrowseDescription{
		NodeID:          nid,
		BrowseDirection: ua.BrowseDirectionForward,
		ReferenceTypeID: ua.NewNumericNodeID(0, id.HierarchicalReferences),
		IncludeSubtypes: true,
		NodeClassMask:   uint32(ua.NodeClassObject | ua.NodeClassVariable | ua.NodeClassObjectType | ua.NodeClassVariableType),
		ResultMask:      uint32(ua.BrowseResultMaskAll),
	}
	refs, err := d.browseAllReferences(ctx, c, desc)
	if err != nil {
		return nil, err
	}
	out := make([]core.BrowseNode, 0, len(refs))
	for _, ref := range refs {
		if ref == nil || ref.NodeID == nil || ref.NodeID.NodeID == nil {
			continue
		}
		bn := core.BrowseNode{
			NodeID:      formatNodeID(ref.NodeID.NodeID),
			BrowseName:  ref.BrowseName.Name,
			DisplayName: ref.DisplayName.Text,
			NodeClass:   fmt.Sprintf("%v", ref.NodeClass),
			IsLeaf:      d.isScalarVariableLeaf(ctx, c, ref),
		}
		if bn.IsLeaf {
			if dt := d.ResolveTagDataType(ctx, bn.NodeID, bn.BrowseName); dt != "" {
				bn.DataType = string(dt)
			}
		}
		out = append(out, bn)
	}
	return out, nil
}

// isScalarVariableLeaf is false for OPC structure/UDT variables that expose field nodes as children.
func (d *Driver) isScalarVariableLeaf(ctx context.Context, c opcuaClient, ref *ua.ReferenceDescription) bool {
	if ref.NodeClass != ua.NodeClassVariable {
		return false
	}
	childDesc := &ua.BrowseDescription{
		NodeID:          ref.NodeID.NodeID,
		BrowseDirection: ua.BrowseDirectionForward,
		ReferenceTypeID: ua.NewNumericNodeID(0, id.HierarchicalReferences),
		IncludeSubtypes: true,
		NodeClassMask:   uint32(ua.NodeClassObject | ua.NodeClassVariable),
		ResultMask:      uint32(ua.BrowseResultMaskAll),
	}
	kids, err := d.browseAllReferences(ctx, c, childDesc)
	if err != nil || len(kids) == 0 {
		return true
	}
	return false
}

// opcuaClient is the subset of *opcua.Client used for browsing.
type opcuaClient interface {
	Browse(context.Context, *ua.BrowseRequest) (*ua.BrowseResponse, error)
	BrowseNext(context.Context, *ua.BrowseNextRequest) (*ua.BrowseNextResponse, error)
}

func (d *Driver) browseAllReferences(ctx context.Context, c opcuaClient, desc *ua.BrowseDescription) ([]*ua.ReferenceDescription, error) {
	req := &ua.BrowseRequest{NodesToBrowse: []*ua.BrowseDescription{desc}}
	resp, err := c.Browse(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(resp.Results) == 0 || resp.Results[0] == nil {
		return nil, nil
	}
	res := resp.Results[0]
	if res.StatusCode != ua.StatusOK {
		return nil, fmt.Errorf("browse status %s", res.StatusCode)
	}
	all := append([]*ua.ReferenceDescription(nil), res.References...)
	cp := res.ContinuationPoint
	for len(cp) > 0 {
		nextResp, err := c.BrowseNext(ctx, &ua.BrowseNextRequest{
			ContinuationPoints:        [][]byte{cp},
			ReleaseContinuationPoints: false,
		})
		if err != nil {
			return nil, err
		}
		if len(nextResp.Results) == 0 || nextResp.Results[0] == nil {
			break
		}
		nres := nextResp.Results[0]
		if nres.StatusCode != ua.StatusOK {
			return nil, fmt.Errorf("browse next status %s", nres.StatusCode)
		}
		all = append(all, nres.References...)
		cp = nres.ContinuationPoint
	}
	return all, nil
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
// Each node is browsed once; children of structures are reused (no double Browse for IsLeaf).
// DataType comes from batched OPC Attribute Read (GuessDataType only if Read fails).
func (d *Driver) ExpandStructure(ctx context.Context, parentNodeID, parentTagID string, maxDepth int) ([]core.ExpandedTag, error) {
	return d.ExpandStructureWithProgress(ctx, parentNodeID, parentTagID, maxDepth, nil)
}

// ExpandProgressFunc reports expand phases: "browse" (done=total=leaf count) then
// "datatype" (done/total while Attribute DataType is read in chunks).
type ExpandProgressFunc func(phase string, done, total int)

// ExpandStructureWithProgress is ExpandStructure plus optional progress callbacks
// for UI (e.g. "Reading datatypes 1200/3500…").
func (d *Driver) ExpandStructureWithProgress(ctx context.Context, parentNodeID, parentTagID string, maxDepth int, onProgress ExpandProgressFunc) ([]core.ExpandedTag, error) {
	if maxDepth <= 0 {
		maxDepth = 16
	}
	if parentTagID == "" {
		parentTagID = "udt"
	}
	var out []core.ExpandedTag
	if err := d.expandWalk(ctx, parentNodeID, parentTagID, "", 0, maxDepth, &out); err != nil {
		return nil, err
	}
	if onProgress != nil {
		onProgress("browse", len(out), len(out))
	}
	d.fillExpandedDataTypes(ctx, out, onProgress)
	return out, nil
}

func (d *Driver) fillExpandedDataTypes(ctx context.Context, tags []core.ExpandedTag, onProgress ExpandProgressFunc) {
	if len(tags) == 0 {
		return
	}
	ids := make([]string, len(tags))
	for i := range tags {
		ids[i] = tags[i].NodeID
	}
	types := d.readOPCDataTypesBatch(ctx, ids, func(done, total int) {
		if onProgress != nil {
			onProgress("datatype", done, total)
		}
	})
	for i := range tags {
		if types[i] != "" {
			tags[i].DataType = types[i]
			continue
		}
		// Last resort only — prefer empty never; poll needs a platform type.
		tags[i].DataType = GuessDataType(browseNameHint(tags[i]))
	}
}

func (d *Driver) expandWalk(ctx context.Context, nodeID, tagPrefix, path string, depth, maxDepth int, out *[]core.ExpandedTag) error {
	if depth > maxDepth {
		return nil
	}
	refs, err := d.browseRefsAt(ctx, nodeID)
	if err != nil {
		return err
	}
	return d.expandFromRefs(ctx, refs, tagPrefix, path, depth, maxDepth, out)
}

func (d *Driver) browseRefsAt(ctx context.Context, parentNodeID string) ([]*ua.ReferenceDescription, error) {
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
	desc := &ua.BrowseDescription{
		NodeID:          nid,
		BrowseDirection: ua.BrowseDirectionForward,
		ReferenceTypeID: ua.NewNumericNodeID(0, id.HierarchicalReferences),
		IncludeSubtypes: true,
		NodeClassMask:   uint32(ua.NodeClassObject | ua.NodeClassVariable | ua.NodeClassObjectType | ua.NodeClassVariableType),
		ResultMask:      uint32(ua.BrowseResultMaskAll),
	}
	return d.browseAllReferences(ctx, c, desc)
}

// expandBrowseBatch caps NodesToBrowse per request (Siemens BadTooManyOperations).
const expandBrowseBatch = 40

func (d *Driver) browseRefsMulti(ctx context.Context, nodeIDs []*ua.NodeID) ([][]*ua.ReferenceDescription, error) {
	d.mu.Lock()
	c := d.client
	d.mu.Unlock()
	if c == nil {
		return nil, fmt.Errorf("not connected")
	}
	out := make([][]*ua.ReferenceDescription, len(nodeIDs))
	for i := 0; i < len(nodeIDs); i += expandBrowseBatch {
		end := i + expandBrowseBatch
		if end > len(nodeIDs) {
			end = len(nodeIDs)
		}
		descs := make([]*ua.BrowseDescription, 0, end-i)
		for _, nid := range nodeIDs[i:end] {
			descs = append(descs, &ua.BrowseDescription{
				NodeID:          nid,
				BrowseDirection: ua.BrowseDirectionForward,
				ReferenceTypeID: ua.NewNumericNodeID(0, id.HierarchicalReferences),
				IncludeSubtypes: true,
				NodeClassMask:   uint32(ua.NodeClassObject | ua.NodeClassVariable | ua.NodeClassObjectType | ua.NodeClassVariableType),
				ResultMask:      uint32(ua.BrowseResultMaskAll),
			})
		}
		batch, err := d.browseManyReferences(ctx, c, descs)
		if err != nil {
			return nil, err
		}
		copy(out[i:end], batch)
	}
	return out, nil
}

// browseManyReferences browses several nodes in one request; ContinuationPoints are drained per result.
func (d *Driver) browseManyReferences(ctx context.Context, c opcuaClient, descs []*ua.BrowseDescription) ([][]*ua.ReferenceDescription, error) {
	if len(descs) == 0 {
		return nil, nil
	}
	req := &ua.BrowseRequest{NodesToBrowse: descs}
	resp, err := c.Browse(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(resp.Results) != len(descs) {
		return nil, fmt.Errorf("browse results %d want %d", len(resp.Results), len(descs))
	}
	out := make([][]*ua.ReferenceDescription, len(descs))
	for i, res := range resp.Results {
		if res == nil {
			continue
		}
		if res.StatusCode != ua.StatusOK {
			return nil, fmt.Errorf("browse status %s", res.StatusCode)
		}
		all := append([]*ua.ReferenceDescription(nil), res.References...)
		cp := res.ContinuationPoint
		for len(cp) > 0 {
			nextResp, err := c.BrowseNext(ctx, &ua.BrowseNextRequest{
				ContinuationPoints:        [][]byte{cp},
				ReleaseContinuationPoints: false,
			})
			if err != nil {
				return nil, err
			}
			if len(nextResp.Results) == 0 || nextResp.Results[0] == nil {
				break
			}
			nres := nextResp.Results[0]
			if nres.StatusCode != ua.StatusOK {
				return nil, fmt.Errorf("browse next status %s", nres.StatusCode)
			}
			all = append(all, nres.References...)
			cp = nres.ContinuationPoint
		}
		out[i] = all
	}
	return out, nil
}

func (d *Driver) expandFromRefs(ctx context.Context, refs []*ua.ReferenceDescription, tagPrefix, path string, depth, maxDepth int, out *[]core.ExpandedTag) error {
	if depth > maxDepth {
		return nil
	}
	type child struct {
		ref      *ua.ReferenceDescription
		name     string
		path     string
		nodeID   string
		tagID    string
		uaNodeID *ua.NodeID
	}
	children := make([]child, 0, len(refs))
	for _, ref := range refs {
		if ref == nil || ref.NodeID == nil || ref.NodeID.NodeID == nil {
			continue
		}
		name := ref.BrowseName.Name
		childPath := name
		if path != "" {
			childPath = path + "." + name
		}
		children = append(children, child{
			ref:      ref,
			name:     name,
			path:     childPath,
			nodeID:   formatNodeID(ref.NodeID.NodeID),
			tagID:    sanitizeTagID(tagPrefix + "_" + childPath),
			uaNodeID: ref.NodeID.NodeID,
		})
	}
	if len(children) == 0 {
		return nil
	}

	uaIDs := make([]*ua.NodeID, len(children))
	for i := range children {
		uaIDs[i] = children[i].uaNodeID
	}
	grandAll, err := d.browseRefsMulti(ctx, uaIDs)
	if err != nil {
		return err
	}

	for i, ch := range children {
		grandRefs := grandAll[i]
		isVar := ch.ref.NodeClass == ua.NodeClassVariable
		if isVar && len(grandRefs) == 0 {
			// DataType filled later via batched OPC Attribute Read.
			*out = append(*out, core.ExpandedTag{
				ID:         ch.tagID,
				NodeID:     ch.nodeID,
				BrowsePath: ch.path,
			})
			continue
		}
		if len(grandRefs) == 0 {
			continue
		}
		if err := d.expandFromRefs(ctx, grandRefs, tagPrefix, ch.path, depth+1, maxDepth, out); err != nil {
			return err
		}
	}
	return nil
}

func formatNodeID(n *ua.NodeID) string {
	if n == nil {
		return ""
	}
	ns := n.Namespace()
	switch n.Type() {
	case ua.NodeIDTypeString:
		return fmt.Sprintf("ns=%d;s=%s", ns, n.StringID())
	case ua.NodeIDTypeGUID:
		return fmt.Sprintf("ns=%d;g=%s", ns, n.StringID())
	case ua.NodeIDTypeByteString:
		return fmt.Sprintf("ns=%d;b=%s", ns, n.StringID())
	default:
		return fmt.Sprintf("ns=%d;i=%d", ns, n.IntID())
	}
}

func sanitizeTagID(s string) string {
	s = strings.ToLower(s)
	repl := strings.NewReplacer(" ", "_", ".", "_", "-", "_", "/", "_")
	return repl.Replace(s)
}

func guessDataType(browseName string) core.ValueType {
	return GuessDataType(browseName)
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
