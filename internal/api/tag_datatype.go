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

// resolveEmptyDataTypesBatch fills empty DataType fields via batched OPC Attribute Read.
func (s *Server) resolveEmptyDataTypesBatch(ctx context.Context, deviceID string, tags []core.Tag) {
	if s.DevHub == nil || len(tags) == 0 {
		return
	}
	var need []int
	for i := range tags {
		if core.NormalizeValueType(tags[i].DataType) == "" || !core.ValidValueType(tags[i].DataType) {
			need = append(need, i)
		}
	}
	if len(need) == 0 {
		return
	}
	ent, ok := s.DevHub.Entry(deviceID)
	if !ok || !ent.Connected {
		for _, i := range need {
			hint := tags[i].ID
			if hint == "" {
				hint = tags[i].NodeID
			}
			tags[i].DataType = opcuaDriver.GuessDataType(hint)
		}
		return
	}
	drv, ok := ent.Driver.(*opcuaDriver.Driver)
	if !ok {
		for _, i := range need {
			tags[i].DataType = opcuaDriver.GuessDataType(tags[i].ID)
		}
		return
	}
	subset := make([]core.Tag, len(need))
	for j, i := range need {
		subset[j] = tags[i]
	}
	opcuaDriver.ApplyDataTypesFromOPC(ctx, drv, subset)
	for j, i := range need {
		tags[i].DataType = subset[j].DataType
	}
}
