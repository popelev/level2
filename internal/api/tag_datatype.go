package api

import (
	"context"

	opcuaDriver "github.com/popelev/level2/internal/driver/opcua"
	"github.com/popelev/level2/internal/core"
)

// resolveTagDataType fills tag.DataType from OPC only when the client left it empty.
// Explicit PUT/UI choices must not be overwritten (sync endpoint refreshes from OPC).
func (s *Server) resolveTagDataType(ctx context.Context, deviceID string, tag *core.Tag) {
	if tag == nil || tag.NodeID == "" || s.DevHub == nil {
		return
	}
	if core.NormalizeValueType(tag.DataType) != "" && core.ValidValueType(tag.DataType) {
		tag.DataType = core.NormalizeValueType(tag.DataType)
		return
	}
	ent, ok := s.DevHub.Entry(deviceID)
	if !ok || !ent.Connected {
		return
	}
	drv, ok := ent.Driver.(*opcuaDriver.Driver)
	if !ok {
		return
	}
	hint := tag.ID
	if hint == "" {
		hint = tag.NodeID
	}
	if dt := drv.ResolveTagDataType(ctx, tag.NodeID, hint); dt != "" {
		tag.DataType = dt
	}
}
