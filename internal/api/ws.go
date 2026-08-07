package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/popelev/level2/internal/core"
)

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
