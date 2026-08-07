package importexcel

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/popelev/level2/internal/core"
	"github.com/xuri/excelize/v2"
)

var exportHeaders = []string{"Area", "Path", "Signal", "MeasurePoint NodeId", "DataType", "DataType Name"}

// Write builds a plant-format .xlsx for the given tags (same schema as Parse).
func Write(tags []core.Tag) ([]byte, error) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	for i, h := range exportHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return nil, err
		}
	}
	for r, t := range tags {
		area, path := splitStoredPath(t.Path)
		row := []any{
			area,
			path,
			t.ID,
			t.NodeID,
			core.OPCTypeCode(t.DataType),
			typeDisplayName(t.DataType),
		}
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return nil, err
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("write xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

func splitStoredPath(p string) (area, path string) {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return "", ""
	}
	i := strings.Index(p, "/")
	if i < 0 {
		return p, ""
	}
	return p[:i], strings.Trim(p[i+1:], "/")
}

func typeDisplayName(dt core.ValueType) string {
	switch dt {
	case core.ValueBool:
		return "Boolean"
	case core.ValueInt64:
		return "Int16"
	case core.ValueUint:
		return "UInt32"
	case core.ValueString:
		return "String"
	case core.ValueDateTime:
		return "DateTime"
	default:
		return "Float"
	}
}

