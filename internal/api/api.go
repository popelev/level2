package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	"github.com/popelev/level2/internal/historian/timescale"
	"github.com/popelev/level2/internal/importexcel"
	devruntime "github.com/popelev/level2/internal/runtime"
	"github.com/popelev/level2/internal/store"
)

// dbBackend is the Timescale historian surface used by HTTP handlers.
// Narrowed from *timescale.Historian so unit tests can inject fakes without a live DB.
type dbBackend interface {
	Status(ctx context.Context, databaseURL string) timescale.ConnectionStatus
	Capacity(ctx context.Context) (timescale.CapacityStats, error)
	CapacityPolicy() timescale.CapacityPolicySettings
	SetCapacityPolicy(percent int, policy string)
	WipeSamples(ctx context.Context) (timescale.WipeResult, error)
	WriteBatch(ctx context.Context, samples []core.Sample) error
	Ping(ctx context.Context) error
}

// Server exposes REST + WS for the collector (M3).
type Server struct {
	Log      *slog.Logger
	Live     *store.Live
	Tags     func() []core.Tag
	Devices  func() []core.Device
	History  core.HistoryQuerier
	DB       dbBackend
	Browser  core.Browser // fallback when device_id omitted
	DevHub   *devruntime.Hub
	Cfg      *config.Store
	Hub      *Hub
	Diag     *diag.Buffer
	// Incidents tracks downtime / failure edges for last-hour counters (optional).
	Incidents *diag.IncidentTracker
	// ReadyCheck mirrors /readyz (optional; falls back to DevHub.AnyConnected).
	ReadyCheck func() bool
	// OPCWriteEnabled gates PUT …/value (optional; falls back to Cfg.OPCWriteEnabled, else false).
	OPCWriteEnabled func() bool
	// TagSimulationActive is true when this process is feeding mock tag samples
	// (tag_simulation / LEVEL2_TAG_SIMULATION or LEVEL2_SIM_BROWSER). Never inferred from disconnect.
	TagSimulationActive func() bool
	// SimBrowserActive is true when LEVEL2_SIM_BROWSER replaced OPC browse with simbrowser.
	SimBrowserActive func() bool
	// APIToken returns the shared API token; empty disables auth (optional; falls back to Cfg.APIToken).
	APIToken func() string
	// OnDeviceChanged is called after device create/update/delete (optional).
	OnDeviceChanged func(deviceID string, removed bool)
}

func (s *Server) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/tags", s.handleTags)
	mux.HandleFunc("GET /api/v1/tags/{id}/value", s.handleTagValue)
	mux.HandleFunc("GET /api/v1/tags/{id}/history", s.handleHistory)
	mux.HandleFunc("GET /api/v1/devices", s.handleDevices)
	mux.HandleFunc("POST /api/v1/devices", s.handleCreateDevice)
	mux.HandleFunc("PUT /api/v1/devices/{id}", s.handleUpdateDevice)
	mux.HandleFunc("DELETE /api/v1/devices/{id}", s.handleDeleteDevice)
	mux.HandleFunc("GET /api/v1/browse", s.handleBrowse)
	mux.HandleFunc("POST /api/v1/expand", s.handleExpand)
	mux.HandleFunc("POST /api/v1/devices/{id}/tags/import", s.handleImportExcel)
	mux.HandleFunc("GET /api/v1/devices/{id}/tags.xlsx", s.handleExportTagsExcel)
	mux.HandleFunc("POST /api/v1/devices/{id}/tags", s.handleUpsertTag)
	mux.HandleFunc("PUT /api/v1/devices/{id}/tags/{tagId}", s.handleUpsertTag)
	mux.HandleFunc("DELETE /api/v1/devices/{id}/tags/{tagId}", s.handleDeleteTag)
	mux.HandleFunc("PUT /api/v1/tags/{id}/value", s.handleWriteTagValue)
	mux.HandleFunc("POST /api/v1/tags/values", s.handleBatchWriteTagValues)
	mux.HandleFunc("GET /api/v1/ws/stream", s.handleWS)
	s.mountProject(mux)
	s.mountDiagnostics(mux)
	s.mountDatabase(mux)
	s.mountTagBulk(mux)
	s.mountTagSimulation(mux)
	s.mountStatus(mux)
	s.mountOpenAPI(mux)
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("device_id")
	devs := s.Devices()
	if deviceID != "" {
		filtered := make([]core.Device, 0, 1)
		for _, d := range devs {
			if d.ID == deviceID {
				filtered = append(filtered, d)
				break
			}
		}
		devs = filtered
	}
	writeJSON(w, http.StatusOK, s.snapshotDevicesForAPI(devs))
}

func (s *Server) handleTagValue(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sample, ok := s.displaySampleForTag(id)
	if !ok {
		http.Error(w, "tag not found or no sample yet", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, sampleDTO(sample))
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if s.History == nil {
		http.Error(w, "history unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	q := r.URL.Query()
	from, _ := time.Parse(time.RFC3339, q.Get("from"))
	to, _ := time.Parse(time.RFC3339, q.Get("to"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	rows, err := s.History.QueryHistory(r.Context(), id, from, to, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]sampleDTOType, 0, len(rows))
	for _, row := range rows {
		out = append(out, sampleDTO(row))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleExportTagsExcel(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store unavailable", http.StatusServiceUnavailable)
		return
	}
	deviceID := r.PathValue("id")
	var tags []core.Tag
	found := false
	for _, d := range s.Cfg.Devices() {
		if d.ID == deviceID {
			tags = d.Tags
			found = true
			break
		}
	}
	if !found {
		http.Error(w, fmt.Sprintf("device %q not found", deviceID), http.StatusNotFound)
		return
	}
	b, err := importexcel.Write(tags)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := fmt.Sprintf("%s-tags.xlsx", deviceID)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	_, _ = w.Write(b)
}

func (s *Server) handleImportExcel(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store unavailable", http.StatusServiceUnavailable)
		return
	}
	deviceID := r.PathValue("id")
	if deviceID == "" {
		http.Error(w, "device id required", http.StatusBadRequest)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "multipart form required (file=*.xlsx)", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	parsed, err := importexcel.Parse(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	replace := r.URL.Query().Get("replace") == "1" || r.FormValue("replace") == "1"
	added, updated, err := s.Cfg.MergeTags(deviceID, parsed.Tags, replace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id": deviceID,
		"added":     added,
		"updated":   updated,
		"total":     len(parsed.Tags),
		"errors":    parsed.Errors,
		"tags":      parsed.Tags,
	})
}

func (s *Server) handleUpsertTag(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store unavailable", http.StatusServiceUnavailable)
		return
	}
	deviceID := r.PathValue("id")
	var tag core.Tag
	if err := json.NewDecoder(r.Body).Decode(&tag); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if tid := r.PathValue("tagId"); tid != "" {
		tag.ID = tid
	}
	s.resolveTagDataType(r.Context(), deviceID, &tag)
	if err := s.Cfg.UpsertTag(deviceID, tag); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, tag)
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store unavailable", http.StatusServiceUnavailable)
		return
	}
	deviceID := r.PathValue("id")
	tagID := r.PathValue("tagId")
	if err := s.Cfg.DeleteTag(deviceID, tagID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
