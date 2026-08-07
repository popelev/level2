package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/popelev/level2/internal/core"
)

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
