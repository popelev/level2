package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	"github.com/popelev/level2/internal/historian/timescale"
	"github.com/popelev/level2/internal/importexcel"
	"github.com/popelev/level2/internal/metrics"
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

// wsClient is one WebSocket subscriber. filter nil = all tags; non-nil = only listed tag_ids.
type wsClient struct {
	conn   *websocket.Conn
	filter map[string]struct{} // nil → no filter (all); empty map → subscribe to none
}

type Hub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]*wsClient
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*websocket.Conn]*wsClient)}
}

func (h *Hub) Broadcast(s core.Sample) {
	h.mu.Lock()
	defer h.mu.Unlock()
	payload, err := json.Marshal(sampleDTO(s))
	if err != nil {
		return
	}
	for c, cl := range h.clients {
		if cl.filter != nil {
			if _, ok := cl.filter[s.TagID]; !ok {
				continue
			}
		}
		_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if err := c.WriteMessage(websocket.TextMessage, payload); err != nil {
			_ = c.Close()
			delete(h.clients, c)
		}
	}
}

func (h *Hub) add(c *websocket.Conn, filter map[string]struct{}) {
	h.mu.Lock()
	h.clients[c] = &wsClient{conn: c, filter: filter}
	h.mu.Unlock()
}

func (h *Hub) setFilter(c *websocket.Conn, filter map[string]struct{}) {
	h.mu.Lock()
	if cl, ok := h.clients[c]; ok {
		cl.filter = filter
	}
	h.mu.Unlock()
}

func (h *Hub) remove(c *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	_ = c.Close()
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

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

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	devs := s.Devices()
	status := map[string]bool{}
	if s.DevHub != nil {
		status = s.DevHub.Status()
	}
	type dto struct {
		ID              string `json:"id"`
		Endpoint        string `json:"endpoint"`
		Security        string `json:"security"`
		Username        string `json:"username,omitempty"`
		PollConcurrency int    `json:"poll_concurrency"`
		TagCount        int    `json:"tag_count"`
		Connected       bool   `json:"connected"`
	}
	out := make([]dto, 0, len(devs))
	for _, d := range devs {
		out = append(out, dto{
			ID: d.ID, Endpoint: d.Endpoint, Security: d.Security,
			Username: d.Username, TagCount: len(d.Tags),
			PollConcurrency: core.NormalizePollConcurrency(d.PollConcurrency),
			Connected:       status[d.ID],
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateDevice(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store unavailable", http.StatusServiceUnavailable)
		return
	}
	var body core.Device
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	for _, d := range s.Cfg.Devices() {
		if d.ID == body.ID {
			http.Error(w, "device already exists", http.StatusConflict)
			return
		}
	}
	if err := s.Cfg.UpsertDevice(body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.OnDeviceChanged != nil {
		s.OnDeviceChanged(body.ID, false)
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": body.ID})
}

func (s *Server) handleUpdateDevice(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	var body core.Device
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	body.ID = id
	if err := s.Cfg.UpsertDevice(body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.OnDeviceChanged != nil {
		s.OnDeviceChanged(id, false)
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if err := s.Cfg.DeleteDevice(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if s.OnDeviceChanged != nil {
		s.OnDeviceChanged(id, true)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resolveBrowser(deviceID string) (core.Browser, error) {
	if deviceID != "" && s.DevHub != nil {
		return s.DevHub.Browser(deviceID)
	}
	if s.Browser != nil {
		return s.Browser, nil
	}
	return nil, fmt.Errorf("browse unavailable")
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("device_id")
	br, err := s.resolveBrowser(deviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	node := r.URL.Query().Get("node_id")
	if node == "" {
		node = "ns=0;i=84" // Root
	}
	nodes, err := br.BrowseChildren(r.Context(), node)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleExpand(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeviceID    string `json:"device_id"`
		NodeID      string `json:"node_id"`
		ParentTagID string `json:"parent_tag_id"`
		MaxDepth    int    `json:"max_depth"`
		Stream      bool   `json:"stream"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.NodeID == "" {
		http.Error(w, "node_id required", http.StatusBadRequest)
		return
	}
	br, err := s.resolveBrowser(body.DeviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	wantStream := body.Stream || strings.Contains(r.Header.Get("Accept"), "ndjson")
	if wantStream {
		s.handleExpandStream(w, r, br, body.NodeID, body.ParentTagID, body.MaxDepth)
		return
	}

	tags, err := br.ExpandStructure(r.Context(), body.NodeID, body.ParentTagID, body.MaxDepth)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

// expandWithProgress is implemented by the OPC driver for NDJSON progress during expand.
type expandWithProgress interface {
	ExpandStructureWithProgress(ctx context.Context, parentNodeID, parentTagID string, maxDepth int, onProgress func(phase string, done, total int)) ([]core.ExpandedTag, error)
}

func (s *Server) handleExpandStream(w http.ResponseWriter, r *http.Request, br core.Browser, nodeID, parentTagID string, maxDepth int) {
	flusher, canFlush := w.(http.Flusher)
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	writeEv := func(v any) {
		_ = enc.Encode(v)
		if canFlush {
			flusher.Flush()
		}
	}

	var tags []core.ExpandedTag
	var err error
	if prog, ok := br.(expandWithProgress); ok {
		tags, err = prog.ExpandStructureWithProgress(r.Context(), nodeID, parentTagID, maxDepth, func(phase string, done, total int) {
			writeEv(map[string]any{
				"type":  "progress",
				"phase": phase,
				"done":  done,
				"total": total,
			})
		})
	} else {
		tags, err = br.ExpandStructure(r.Context(), nodeID, parentTagID, maxDepth)
		if err == nil {
			writeEv(map[string]any{"type": "progress", "phase": "browse", "done": len(tags), "total": len(tags)})
		}
	}
	if err != nil {
		writeEv(map[string]any{"type": "error", "error": err.Error()})
		return
	}
	writeEv(map[string]any{"type": "result", "tags": tags})
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

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	filter := parseWSTagFilter(r)
	s.Hub.add(c, filter)
	defer s.Hub.remove(c)
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			return
		}
		// Optional client control: {"subscribe":["tag_a","tag_b"]} replaces the filter.
		// Empty subscribe list means receive nothing; omit subscribe to keep current filter.
		if len(data) == 0 {
			continue
		}
		var msg struct {
			Subscribe *[]string `json:"subscribe"`
			TagIDs    *[]string `json:"tag_ids"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		var ids []string
		if msg.Subscribe != nil {
			ids = *msg.Subscribe
		} else if msg.TagIDs != nil {
			ids = *msg.TagIDs
		} else {
			continue
		}
		s.Hub.setFilter(c, tagIDSet(ids))
	}
}

// parseWSTagFilter reads ?tag_id=a&tag_id=b and/or ?tag_ids=a,b.
// nil = no filter (all tags). Non-nil map (possibly empty) = allow-list.
func parseWSTagFilter(r *http.Request) map[string]struct{} {
	q := r.URL.Query()
	var ids []string
	ids = append(ids, q["tag_id"]...)
	if raw := q.Get("tag_ids"); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				ids = append(ids, p)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return tagIDSet(ids)
}

func tagIDSet(ids []string) map[string]struct{} {
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		out[id] = struct{}{}
	}
	return out
}

type sampleDTOType struct {
	Time      time.Time `json:"time"`
	TagID     string    `json:"tag_id"`
	ValueNum  *float64  `json:"value_num,omitempty"`
	ValueText *string   `json:"value_text,omitempty"`
	ValueBool *bool     `json:"value_bool,omitempty"`
	Quality   int       `json:"quality"`
}

func sampleDTO(s core.Sample) sampleDTOType {
	return sampleDTOType{
		Time: s.Time, TagID: s.TagID,
		ValueNum: s.ValueNum, ValueText: s.ValueText, ValueBool: s.ValueBool,
		Quality: int(s.Quality),
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// FanIn updates live store and broadcasts WS from sample stream.
// Historian (out) receives a sample only when value or quality changed vs Live
// (or the tag has no previous sample). Live and WS update on every sample.
func FanIn(ctx context.Context, in <-chan core.Sample, live *store.Live, hub *Hub, out chan<- core.Sample) {
	for {
		select {
		case <-ctx.Done():
			return
		case s, ok := <-in:
			if !ok {
				return
			}
			prev, hasPrev := live.Get(s.TagID)
			// Bad/uncertain without a new value (link loss, skip): keep last value, flip quality only.
			if hasPrev && s.Quality != core.QualityGood &&
				s.ValueNum == nil && s.ValueText == nil && s.ValueBool == nil {
				merged := prev
				merged.Time = s.Time
				merged.Quality = s.Quality
				s = merged
			}
			unchanged := hasPrev && prev.SamePayload(s)
			live.Update(s)
			hub.Broadcast(s)
			if unchanged {
				metrics.SamplesSuppressedUnchanged.Inc()
				continue
			}
			select {
			case out <- s:
			case <-ctx.Done():
				return
			}
		}
	}
}
