package api

import (
	"net/http"
	"strings"
)

// maxBatchLiveReads mirrors write batch cap (Siemens-scale request size).
const maxBatchLiveReads = 100

// handleBatchTagValues returns live Sample DTOs for a selected tag_id list.
// GET /api/v1/tags/values?id=a&id=b  and/or  ?ids=a,b  /  ?tag_ids=a,b
// Unknown tags / no sample yet are omitted (partial success). Max 100 ids.
func (s *Server) handleBatchTagValues(w http.ResponseWriter, r *http.Request) {
	ids := parseBatchLiveTagIDs(r)
	if len(ids) == 0 {
		http.Error(w, "at least one id required (?id= / ?ids= / ?tag_ids=)", http.StatusBadRequest)
		return
	}
	if len(ids) > maxBatchLiveReads {
		http.Error(w, "too many ids (max 100)", http.StatusBadRequest)
		return
	}
	out := make([]sampleDTOType, 0, len(ids))
	for _, id := range ids {
		sample, ok := s.displaySampleForTag(id)
		if !ok {
			continue
		}
		out = append(out, sampleDTO(sample))
	}
	writeJSON(w, http.StatusOK, out)
}

// parseBatchLiveTagIDs collects unique tag ids in first-seen order from query params.
func parseBatchLiveTagIDs(r *http.Request) []string {
	q := r.URL.Query()
	var raw []string
	raw = append(raw, q["id"]...)
	raw = append(raw, q["tag_id"]...)
	for _, key := range []string{"ids", "tag_ids"} {
		if v := q.Get(key); v != "" {
			for _, p := range strings.Split(v, ",") {
				raw = append(raw, p)
			}
		}
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
