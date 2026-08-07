package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/popelev/level2/internal/core"
)

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
