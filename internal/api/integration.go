package api

import (
	"net/http"
	"time"

	"github.com/popelev/level2/internal/core"
)

// OpenAPIVersion is the canonical contract version (must match api/openapi.yaml info.version).
const OpenAPIVersion = "1.4.0"

type tagCatalogDeviceDTO struct {
	ID        string `json:"id"`
	Connected bool   `json:"connected"`
}

type tagCatalogTagDTO struct {
	TagID      string `json:"tag_id"`
	DeviceID   string `json:"device_id"`
	Path       string `json:"path,omitempty"`
	Datatype   string `json:"datatype"`
	Enabled    bool   `json:"enabled"`
	Writable   bool   `json:"writable"`
	IntervalMs int    `json:"interval_ms"`
	NodeID     string `json:"node_id"`
}

type tagCatalogDTO struct {
	ExportedAt       time.Time             `json:"exported_at"`
	Level2APIVersion string                `json:"level2_api_version"`
	Devices          []tagCatalogDeviceDTO `json:"devices"`
	Tags             []tagCatalogTagDTO    `json:"tags"`
}

// handleIntegrationTagCatalog is a stable read-only export for external math-model
// and similar consumers: flat tag rows without nested live samples.
// GET /api/v1/integration/tag-catalog — same auth as other read GETs (open / any token).
func (s *Server) handleIntegrationTagCatalog(w http.ResponseWriter, r *http.Request) {
	devs := []core.Device{}
	if s.Devices != nil {
		devs = s.Devices()
	}
	status := map[string]bool{}
	if s.DevHub != nil {
		status = s.DevHub.Status()
	}

	devicesOut := make([]tagCatalogDeviceDTO, 0, len(devs))
	tagsOut := make([]tagCatalogTagDTO, 0)
	for _, d := range devs {
		devicesOut = append(devicesOut, tagCatalogDeviceDTO{
			ID:        d.ID,
			Connected: status[d.ID],
		})
		for _, t := range d.Tags {
			tagsOut = append(tagsOut, tagCatalogTagDTO{
				TagID:      t.ID,
				DeviceID:   d.ID,
				Path:       t.Path,
				Datatype:   string(t.DataType),
				Enabled:    t.Enabled,
				Writable:   t.Writable,
				IntervalMs: t.IntervalMs,
				NodeID:     t.NodeID,
			})
		}
	}

	writeJSON(w, http.StatusOK, tagCatalogDTO{
		ExportedAt:       time.Now().UTC().Truncate(time.Second),
		Level2APIVersion: OpenAPIVersion,
		Devices:          devicesOut,
		Tags:             tagsOut,
	})
}
