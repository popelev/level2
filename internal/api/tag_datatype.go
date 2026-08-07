package api

import (
	"context"

	"github.com/popelev/level2/internal/core"
)

// resolveTagDataType fills tag.DataType from OPC only when the client left it empty.
// Explicit PUT/UI choices must not be overwritten — use POST .../tags/sync to refresh from OPC
// (sync always overwrites, including wrong float64 → datetime).
func (s *Server) resolveTagDataType(ctx context.Context, deviceID string, tag *core.Tag) {
	if tag == nil || tag.NodeID == "" {
		return
	}
	// Normalize aliases (float→float64) even when OPC hub is unavailable.
	if core.NormalizeValueType(tag.DataType) != "" && core.ValidValueType(tag.DataType) {
		tag.DataType = core.NormalizeValueType(tag.DataType)
		return
	}
	resolver := s.dataTypeResolver(deviceID)
	if resolver == nil {
		return
	}
	hint := tag.ID
	if hint == "" {
		hint = tag.NodeID
	}
	if dt := resolver.ResolveTagDataType(ctx, tag.NodeID, hint); dt != "" {
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
	resolver := s.dataTypeResolver(deviceID)
	if resolver == nil {
		for _, i := range need {
			hint := tags[i].ID
			if hint == "" {
				hint = tags[i].NodeID
			}
			tags[i].DataType = core.GuessDataType(hint)
		}
		return
	}
	subset := make([]core.Tag, len(need))
	for j, i := range need {
		subset[j] = tags[i]
	}
	resolver.ApplyDataTypes(ctx, subset)
	for j, i := range need {
		tags[i].DataType = subset[j].DataType
	}
}
