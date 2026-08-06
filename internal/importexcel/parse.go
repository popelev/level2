package importexcel

import (
	"fmt"
	"io"
	"strings"

	"github.com/popelev/level2/internal/core"
	"github.com/xuri/excelize/v2"
)

// Row is one imported OPC point from the workbook.
type Row struct {
	Area     string `json:"area,omitempty"`
	Path     string `json:"path,omitempty"`
	Signal   string `json:"signal"`
	NodeID   string `json:"node_id"`
	DataType string `json:"datatype"`
	TypeName string `json:"datatype_name,omitempty"`
}

// Result is the parse outcome.
type Result struct {
	Tags   []core.Tag `json:"tags"`
	Rows   []Row      `json:"rows"`
	Errors []string   `json:"errors,omitempty"`
}

// Parse reads an .xlsx with columns:
// Area | Path | Signal | MeasurePoint NodeId | DataType | DataType Name
func Parse(r io.Reader) (Result, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return Result{}, fmt.Errorf("open xlsx: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return Result{}, fmt.Errorf("workbook has no sheets")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return Result{}, err
	}
	if len(rows) < 2 {
		return Result{}, fmt.Errorf("sheet is empty")
	}

	col := mapHeaders(rows[0])
	if col["signal"] < 0 || col["node_id"] < 0 {
		return Result{}, fmt.Errorf("required columns Signal and MeasurePoint NodeId not found (got %v)", rows[0])
	}

	out := Result{}
	seen := map[string]bool{}
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		signal := cell(row, col["signal"])
		nodeID := cell(row, col["node_id"])
		if signal == "" && nodeID == "" {
			continue
		}
		rec := Row{
			Area:     cell(row, col["area"]),
			Path:     cell(row, col["path"]),
			Signal:   signal,
			NodeID:   nodeID,
			DataType: cell(row, col["datatype"]),
			TypeName: cell(row, col["datatype_name"]),
		}
		tagID := sanitizeID(rec.Signal)
		if tagID == "" {
			out.Errors = append(out.Errors, fmt.Sprintf("row %d: empty Signal", i+1))
			continue
		}
		if _, err := core.ParseNodeID(rec.NodeID); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("row %d (%s): %v", i+1, tagID, err))
			continue
		}
		dt := mapDataType(rec.TypeName, rec.DataType)
		if dt == "" {
			out.Errors = append(out.Errors, fmt.Sprintf("row %d (%s): unsupported datatype %q / %q", i+1, tagID, rec.TypeName, rec.DataType))
			continue
		}
		if seen[tagID] {
			out.Errors = append(out.Errors, fmt.Sprintf("row %d: duplicate signal %q (skipped)", i+1, tagID))
			continue
		}
		seen[tagID] = true
		out.Rows = append(out.Rows, rec)
		out.Tags = append(out.Tags, core.Tag{
			ID:         tagID,
			NodeID:     rec.NodeID,
			Path:       joinPath(rec.Area, rec.Path),
			DataType:   dt,
			Enabled:    true,
			IntervalMs: 1000,
		})
	}
	if len(out.Tags) == 0 {
		return out, fmt.Errorf("no valid tags parsed (%d errors)", len(out.Errors))
	}
	return out, nil
}

func mapHeaders(header []string) map[string]int {
	idx := map[string]int{
		"area": -1, "path": -1, "signal": -1, "node_id": -1, "datatype": -1, "datatype_name": -1,
	}
	for i, h := range header {
		key := normalizeHeader(h)
		switch key {
		case "area":
			idx["area"] = i
		case "path":
			idx["path"] = i
		case "signal":
			idx["signal"] = i
		case "measurepointnodeid", "nodeid", "measurepoint nodeid":
			idx["node_id"] = i
		case "datatype":
			// prefer name column if both exist; index datatype code here first
			if idx["datatype"] < 0 {
				idx["datatype"] = i
			}
		case "datatypename":
			idx["datatype_name"] = i
		}
	}
	// If both DataType and DataType Name exist, excel has DataType as OPC id (i=10) and Name as Float.
	// Our header mapper may assign first "datatype" to code column — good.
	return idx
}

func normalizeHeader(h string) string {
	h = strings.TrimSpace(strings.ToLower(h))
	h = strings.ReplaceAll(h, " ", "")
	h = strings.ReplaceAll(h, "_", "")
	return h
}

func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func sanitizeID(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "_")
	return s
}

func joinPath(parts ...string) string {
	var segs []string
	for _, p := range parts {
		p = strings.Trim(strings.TrimSpace(p), "/")
		if p == "" {
			continue
		}
		segs = append(segs, p)
	}
	return strings.Join(segs, "/")
}

func mapDataType(typeName, typeCode string) core.ValueType {
	n := strings.ToLower(strings.TrimSpace(typeName))
	switch n {
	case "float", "double", "float32", "float64":
		return core.ValueFloat64
	case "boolean", "bool":
		return core.ValueBool
	case "string", "bytestring":
		return core.ValueString
	case "datetime", "date_time", "datetim", "timestamp", "utctime":
		return core.ValueDateTime
	case "int16", "int32", "int64", "sbyte", "byte", "uint16", "uint32", "integer", "int":
		return core.ValueInt64
	}
	// Fallback on OPC UA type NodeId like i=10 (Float), i=1 (Boolean), i=4 (Int16)
	switch strings.TrimSpace(typeCode) {
	case "i=1":
		return core.ValueBool
	case "i=4", "i=6", "i=8", "i=2", "i=3", "i=5", "i=7":
		return core.ValueInt64
	case "i=10", "i=11":
		return core.ValueFloat64
	case "i=12":
		return core.ValueString
	case "i=13":
		return core.ValueDateTime
	}
	return core.NormalizeValueType(core.ValueType(n))
}
