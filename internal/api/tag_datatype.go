package api

import (
	"context"

	opcuaDriver "github.com/popelev/level2/internal/driver/opcua"
	"github.com/popelev/level2/internal/core"
)

func (s *Server) resolveTagDataType(ctx context.Context, deviceID string, tag *core.Tag) {
	if tag == nil || tag.NodeID == "" || s.DevHub == nil {
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
	tag.DataType = drv.ResolveTagDataType(ctx, tag.NodeID, hint)
}
