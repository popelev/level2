package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/projectxlsx"
)

type nodeProber interface {
	ProbeNode(ctx context.Context, nodeID string) (exists bool, isLeaf bool, err error)
}

func (s *Server) mountProject(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/project.xlsx", s.handleProjectExport)
	mux.HandleFunc("POST /api/v1/project/preview", s.handleProjectPreview)
	mux.HandleFunc("POST /api/v1/project/import", s.handleProjectImport)
	mux.HandleFunc("POST /api/v1/project/validate", s.handleProjectValidate)
	mux.HandleFunc("POST /api/v1/project/compare", s.handleProjectCompare)
	mux.HandleFunc("POST /api/v1/project/compare.xlsx", s.handleProjectCompareXLSX)
}

func (s *Server) handleProjectExport(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store unavailable", http.StatusServiceUnavailable)
		return
	}
	b, err := projectxlsx.Write(s.Cfg.Devices())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="level2-project.xlsx"`)
	_, _ = w.Write(b)
}

func (s *Server) parseProjectUpload(r *http.Request) (projectxlsx.Project, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return projectxlsx.Project{}, fmt.Errorf("multipart: %w", err)
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		file, _, err = r.FormFile("a")
	}
	if err != nil {
		return projectxlsx.Project{}, fmt.Errorf("file required")
	}
	defer file.Close()
	return projectxlsx.Parse(file)
}

func (s *Server) handleProjectPreview(w http.ResponseWriter, r *http.Request) {
	p, err := s.parseProjectUpload(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	srv, tags := projectxlsx.Summary(p.Devices)
	writeJSON(w, http.StatusOK, map[string]any{
		"legacy":       p.Legacy,
		"servers":      srv,
		"tags":         tags,
		"legacy_tags":  len(p.LegacyTags),
		"errors":       p.Errors,
		"device_ids":   deviceIDs(p.Devices),
	})
}

func (s *Server) handleProjectImport(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store unavailable", http.StatusServiceUnavailable)
		return
	}
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "merge"
	}
	replace := mode == "replace"
	p, err := s.parseProjectUpload(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if p.Legacy {
		http.Error(w, "legacy plant Excel: import on Import / Export tab for a selected server", http.StatusBadRequest)
		return
	}
	if err := s.Cfg.ApplyProject(p.Devices, replace); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.OnDeviceChanged != nil {
		for _, d := range p.Devices {
			s.OnDeviceChanged(d.ID, false)
		}
	}
	srv, tags := projectxlsx.Summary(p.Devices)
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":    mode,
		"servers": srv,
		"tags":    tags,
		"errors":  p.Errors,
	})
}

type validateRow struct {
	DeviceID     string `json:"device_id"`
	TagID        string `json:"id"`
	NodeID       string `json:"node_id"`
	DataType     string `json:"datatype"`
	ActualType   string `json:"actual_datatype,omitempty"`
	Status       string `json:"status"`
	Detail       string `json:"detail,omitempty"`
}

func inferSampleType(s core.Sample) string {
	if s.ValueBool != nil {
		return string(core.ValueBool)
	}
	if s.ValueText != nil {
		return string(core.ValueString)
	}
	if s.ValueNum != nil {
		// historian stores ints as float too — treat as float64 unless whole
		return string(core.ValueFloat64)
	}
	return ""
}

func (s *Server) handleProjectValidate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeviceID string `json:"device_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	devs := s.Devices()
	if body.DeviceID != "" {
		filtered := make([]core.Device, 0, 1)
		for _, d := range devs {
			if d.ID == body.DeviceID {
				filtered = append(filtered, d)
			}
		}
		devs = filtered
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	var rows []validateRow
	counts := map[string]int{}
	for _, d := range devs {
		br, err := s.resolveBrowser(d.ID)
		prober, _ := br.(nodeProber)
		for _, t := range d.Tags {
			row := validateRow{DeviceID: d.ID, TagID: t.ID, NodeID: t.NodeID, DataType: string(t.DataType)}
			if !t.Enabled {
				row.Status = "disabled_skip"
				rows = append(rows, row)
				counts[row.Status]++
				continue
			}
			if _, err := core.ParseNodeID(t.NodeID); err != nil {
				row.Status = "bad_node_id"
				row.Detail = err.Error()
				rows = append(rows, row)
				counts[row.Status]++
				continue
			}
			if err != nil || br == nil {
				row.Status = "unreachable"
				row.Detail = "browse unavailable"
				rows = append(rows, row)
				counts[row.Status]++
				continue
			}
			if prober == nil {
				row.Status = "unreachable"
				row.Detail = "probe not supported"
				rows = append(rows, row)
				counts[row.Status]++
				continue
			}
			exists, isLeaf, perr := prober.ProbeNode(ctx, t.NodeID)
			if perr != nil {
				row.Status = "unreachable"
				row.Detail = perr.Error()
			} else if !exists {
				row.Status = "missing"
			} else if !isLeaf {
				row.Status = "not_variable"
			} else {
				row.Status = "ok"
				if s.Live != nil {
					if sample, ok := s.Live.Get(t.ID); ok {
						actual := inferSampleType(sample)
						row.ActualType = actual
						if actual != "" && string(t.DataType) == string(core.ValueBool) && actual != string(core.ValueBool) {
							row.Status = "type_mismatch"
							row.Detail = "expected bool, sample is " + actual
						} else if actual != "" && string(t.DataType) == string(core.ValueString) && actual != string(core.ValueString) {
							row.Status = "type_mismatch"
							row.Detail = "expected string, sample is " + actual
						} else if actual == string(core.ValueBool) && t.DataType != core.ValueBool {
							row.Status = "type_mismatch"
							row.Detail = "sample is bool, tag is " + string(t.DataType)
						} else if actual == string(core.ValueString) && t.DataType != core.ValueString {
							row.Status = "type_mismatch"
							row.Detail = "sample is string, tag is " + string(t.DataType)
						}
					}
				}
			}
			rows = append(rows, row)
			counts[row.Status]++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows, "counts": counts})
}

func (s *Server) loadCompareSides(r *http.Request) (a, b []core.Device, err error) {
	sideA := r.URL.Query().Get("a")
	sideB := r.URL.Query().Get("b")
	if sideA == "" {
		sideA = "live"
	}
	if sideB == "" {
		sideB = "file"
	}
	_ = r.ParseMultipartForm(32 << 20)
	load := func(side, field string) ([]core.Device, error) {
		if side == "live" {
			if s.Cfg == nil {
				return nil, fmt.Errorf("live config unavailable")
			}
			return s.Cfg.Devices(), nil
		}
		file, _, err := r.FormFile(field)
		if err != nil {
			return nil, fmt.Errorf("%s file required", field)
		}
		defer file.Close()
		p, err := projectxlsx.Parse(file)
		if err != nil {
			return nil, err
		}
		if p.Legacy {
			return nil, fmt.Errorf("%s: legacy plant sheet not supported for compare (use Project.xlsx)", field)
		}
		return p.Devices, nil
	}
	a, err = load(sideA, "a")
	if err != nil {
		return nil, nil, err
	}
	b, err = load(sideB, "b")
	if err != nil {
		return nil, nil, err
	}
	return a, b, nil
}

func (s *Server) handleProjectCompare(w http.ResponseWriter, r *http.Request) {
	a, b, err := s.loadCompareSides(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rows := projectxlsx.Compare(a, b)
	writeJSON(w, http.StatusOK, map[string]any{
		"rows":  rows,
		"count": len(rows),
	})
}

func (s *Server) handleProjectCompareXLSX(w http.ResponseWriter, r *http.Request) {
	a, b, err := s.loadCompareSides(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rows := projectxlsx.Compare(a, b)
	srv, tags := projectxlsx.DiffToSheets(rows)
	out, err := projectxlsx.DiffExcel(srv, tags)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="level2-diff.xlsx"`)
	_, _ = w.Write(out)
}

func deviceIDs(devs []core.Device) []string {
	out := make([]string, 0, len(devs))
	for _, d := range devs {
		out = append(out, d.ID)
	}
	return out
}
