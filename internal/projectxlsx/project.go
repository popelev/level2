package projectxlsx

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/importexcel"
	"github.com/xuri/excelize/v2"
)

const (
	SheetServers = "Servers"
	SheetTags    = "Tags"
)

// Project is a portable snapshot of devices + monitored tags (no passwords).
type Project struct {
	Devices []core.Device
	Errors  []string
	Legacy  bool // true when file was a plant tag sheet (needs target device)
	LegacyTags []core.Tag
}

// Write builds Project.xlsx bytes from devices.
func Write(devices []core.Device) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	_ = f.SetSheetName(f.GetSheetName(0), SheetServers)
	srvHeaders := []string{"id", "endpoint", "username", "security", "connected"}
	for i, h := range srvHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(SheetServers, cell, h)
	}
	for r, d := range devices {
		row := r + 2
		_ = f.SetCellValue(SheetServers, cell(1, row), d.ID)
		_ = f.SetCellValue(SheetServers, cell(2, row), d.Endpoint)
		_ = f.SetCellValue(SheetServers, cell(3, row), d.Username)
		_ = f.SetCellValue(SheetServers, cell(4, row), d.Security)
	}

	_, err := f.NewSheet(SheetTags)
	if err != nil {
		return nil, err
	}
	tagHeaders := []string{"device_id", "id", "path", "node_id", "datatype", "enabled", "interval_ms", "writable", "simulate"}
	for i, h := range tagHeaders {
		_ = f.SetCellValue(SheetTags, cell(i+1, 1), h)
	}
	tr := 2
	for _, d := range devices {
		for _, t := range d.Tags {
			_ = f.SetCellValue(SheetTags, cell(1, tr), d.ID)
			_ = f.SetCellValue(SheetTags, cell(2, tr), t.ID)
			_ = f.SetCellValue(SheetTags, cell(3, tr), t.Path)
			_ = f.SetCellValue(SheetTags, cell(4, tr), t.NodeID)
			_ = f.SetCellValue(SheetTags, cell(5, tr), string(t.DataType))
			_ = f.SetCellValue(SheetTags, cell(6, tr), t.Enabled)
			_ = f.SetCellValue(SheetTags, cell(7, tr), t.IntervalMs)
			_ = f.SetCellValue(SheetTags, cell(8, tr), t.Writable)
			_ = f.SetCellValue(SheetTags, cell(9, tr), t.Simulate)
			tr++
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Parse reads Project.xlsx or legacy plant tag sheet.
func Parse(r io.Reader) (Project, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return Project{}, err
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return Project{}, fmt.Errorf("open xlsx: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return Project{}, fmt.Errorf("workbook has no sheets")
	}

	hasServers := sheetNamed(sheets, SheetServers)
	hasTags := sheetNamed(sheets, SheetTags)
	if hasServers || hasTags {
		return parseProject(f)
	}

	// Legacy plant format via importexcel
	res, err := importexcel.Parse(bytes.NewReader(raw))
	if err != nil {
		return Project{}, err
	}
	return Project{
		Legacy:     true,
		LegacyTags: res.Tags,
		Errors:     res.Errors,
	}, nil
}

func parseProject(f *excelize.File) (Project, error) {
	out := Project{}
	byID := map[string]*core.Device{}

	if name := findSheet(f, SheetServers); name != "" {
		if rows, err := f.GetRows(name); err == nil && len(rows) >= 2 {
			col := mapHeaders(rows[0])
			for i := 1; i < len(rows); i++ {
				row := rows[i]
				id := cellAt(row, col["id"])
				if id == "" {
					continue
				}
				dev := &core.Device{
					ID:       id,
					Endpoint: cellAt(row, col["endpoint"]),
					Username: cellAt(row, col["username"]),
					Security: cellAt(row, col["security"]),
					Tags:     []core.Tag{},
				}
				if dev.Security == "" {
					dev.Security = "None"
				}
				if dev.Endpoint == "" {
					out.Errors = append(out.Errors, fmt.Sprintf("server %q: empty endpoint", id))
					continue
				}
				byID[id] = dev
			}
		}
	}

	if name := findSheet(f, SheetTags); name != "" {
		if rows, err := f.GetRows(name); err == nil && len(rows) >= 2 {
			col := mapHeaders(rows[0])
			for i := 1; i < len(rows); i++ {
				row := rows[i]
				devID := cellAt(row, col["device_id"])
				tagID := cellAt(row, col["id"])
				nodeID := cellAt(row, col["node_id"])
				if tagID == "" && nodeID == "" {
					continue
				}
				if devID == "" {
					out.Errors = append(out.Errors, fmt.Sprintf("tags row %d: missing device_id", i+1))
					continue
				}
				if tagID == "" {
					out.Errors = append(out.Errors, fmt.Sprintf("tags row %d: missing id", i+1))
					continue
				}
				if _, err := core.ParseNodeID(nodeID); err != nil {
					out.Errors = append(out.Errors, fmt.Sprintf("tag %s: %v", tagID, err))
					continue
				}
				dt := core.NormalizeValueType(core.ValueType(strings.ToLower(cellAt(row, col["datatype"]))))
				if dt == "" {
					dt = core.ValueFloat64
				}
				if !core.ValidValueType(dt) {
					out.Errors = append(out.Errors, fmt.Sprintf("tag %s: bad datatype %q", tagID, dt))
					continue
				}
				en := parseBool(cellAt(row, col["enabled"]), true)
				iv, _ := strconv.Atoi(cellAt(row, col["interval_ms"]))
				if iv <= 0 {
					iv = 1000
				}
				wr := parseBool(cellAt(row, col["writable"]), false)
				sim := parseBool(cellAt(row, col["simulate"]), false)
				dev := byID[devID]
				if dev == nil {
					dev = &core.Device{ID: devID, Endpoint: "opc.tcp://unknown", Security: "None", Tags: nil}
					byID[devID] = dev
					out.Errors = append(out.Errors, fmt.Sprintf("tag %s: device %q missing on Servers sheet (placeholder endpoint)", tagID, devID))
				}
				dev.Tags = append(dev.Tags, core.Tag{
					ID:         tagID,
					NodeID:     nodeID,
					Path:       cellAt(row, col["path"]),
					DataType:   dt,
					Enabled:    en,
					IntervalMs: iv,
					Writable:   wr,
					Simulate:   sim,
				})
			}
		}
	}

	for _, d := range byID {
		out.Devices = append(out.Devices, *d)
	}
	if len(out.Devices) == 0 {
		return out, fmt.Errorf("no servers/tags parsed (%d errors)", len(out.Errors))
	}
	return out, nil
}

func findSheet(f *excelize.File, want string) string {
	for _, s := range f.GetSheetList() {
		if strings.EqualFold(s, want) {
			return s
		}
	}
	return ""
}

func sheetNamed(list []string, name string) bool {
	for _, s := range list {
		if strings.EqualFold(s, name) {
			return true
		}
	}
	return false
}

func cell(col, row int) string {
	c, _ := excelize.CoordinatesToCellName(col, row)
	return c
}

func mapHeaders(header []string) map[string]int {
	idx := map[string]int{
		"id": -1, "endpoint": -1, "username": -1, "security": -1,
		"device_id": -1, "path": -1, "node_id": -1, "datatype": -1,
		"enabled": -1, "interval_ms": -1, "writable": -1,
	}
	for i, h := range header {
		key := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(h), " ", "_"))
		switch key {
		case "id", "endpoint", "username", "security", "path", "datatype", "enabled", "writable":
			idx[key] = i
		case "device_id", "deviceid":
			idx["device_id"] = i
		case "node_id", "nodeid":
			idx["node_id"] = i
		case "interval_ms", "intervalms":
			idx["interval_ms"] = i
		}
	}
	return idx
}

func cellAt(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func parseBool(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	case "":
		return def
	default:
		return def
	}
}

// DiffExcel writes Servers_diff + Tags_diff sheets.
func DiffExcel(serverRows, tagRows [][]string) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	_ = f.SetSheetName(f.GetSheetName(0), "Servers_diff")
	for r, row := range serverRows {
		for c, v := range row {
			_ = f.SetCellValue("Servers_diff", cell(c+1, r+1), v)
		}
	}
	_, _ = f.NewSheet("Tags_diff")
	for r, row := range tagRows {
		for c, v := range row {
			_ = f.SetCellValue("Tags_diff", cell(c+1, r+1), v)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
