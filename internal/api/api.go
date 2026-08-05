package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/store"
)

// Server exposes REST + WS for the collector (M3).
type Server struct {
	Log      *slog.Logger
	Live     *store.Live
	Tags     func() []core.Tag
	Devices  func() []core.Device
	History  core.HistoryQuerier
	Browser  core.Browser // optional; nil → 503 on browse/expand
	Hub      *Hub
}

type Hub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*websocket.Conn]struct{})}
}

func (h *Hub) Broadcast(s core.Sample) {
	h.mu.Lock()
	defer h.mu.Unlock()
	payload, err := json.Marshal(sampleDTO(s))
	if err != nil {
		return
	}
	for c := range h.clients {
		_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if err := c.WriteMessage(websocket.TextMessage, payload); err != nil {
			_ = c.Close()
			delete(h.clients, c)
		}
	}
}

func (h *Hub) add(c *websocket.Conn) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
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
	mux.HandleFunc("GET /api/v1/browse", s.handleBrowse)
	mux.HandleFunc("POST /api/v1/expand", s.handleExpand)
	mux.HandleFunc("PUT /api/v1/tags/{id}/value", s.handleWriteNotImplemented)
	mux.HandleFunc("GET /api/v1/ws/stream", s.handleWS)
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	tags := s.Tags()
	writeJSON(w, http.StatusOK, s.Live.SnapshotTags(tags))
}

func (s *Server) handleTagValue(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sample, ok := s.Live.Get(id)
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
	type dto struct {
		ID       string `json:"id"`
		Endpoint string `json:"endpoint"`
		Security string `json:"security"`
		TagCount int    `json:"tag_count"`
	}
	out := make([]dto, 0, len(devs))
	for _, d := range devs {
		out = append(out, dto{ID: d.ID, Endpoint: d.Endpoint, Security: d.Security, TagCount: len(d.Tags)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if s.Browser == nil {
		http.Error(w, "browse unavailable (OPC not connected)", http.StatusServiceUnavailable)
		return
	}
	node := r.URL.Query().Get("node_id")
	if node == "" {
		node = "ns=0;i=85" // Objects folder
	}
	nodes, err := s.Browser.BrowseChildren(r.Context(), node)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleExpand(w http.ResponseWriter, r *http.Request) {
	if s.Browser == nil {
		http.Error(w, "expand unavailable (OPC not connected)", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		NodeID      string `json:"node_id"`
		ParentTagID string `json:"parent_tag_id"`
		MaxDepth    int    `json:"max_depth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.NodeID == "" {
		http.Error(w, "node_id required", http.StatusBadRequest)
		return
	}
	tags, err := s.Browser.ExpandStructure(r.Context(), body.NodeID, body.ParentTagID, body.MaxDepth)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleWriteNotImplemented(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "write to PLC not implemented yet", http.StatusNotImplemented)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.Hub.add(c)
	defer s.Hub.remove(c)
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			return
		}
	}
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
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// FanIn updates live store and broadcasts WS from sample stream.
func FanIn(ctx context.Context, in <-chan core.Sample, live *store.Live, hub *Hub, out chan<- core.Sample) {
	for {
		select {
		case <-ctx.Done():
			return
		case s, ok := <-in:
			if !ok {
				return
			}
			live.Update(s)
			hub.Broadcast(s)
			select {
			case out <- s:
			case <-ctx.Done():
				return
			}
		}
	}
}
