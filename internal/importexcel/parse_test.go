package importexcel

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestParse_TankhouseLike(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	headers := []string{"Area", "Path", "Signal", "MeasurePoint NodeId", "DataType", "DataType Name"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	rows := [][]any{
		{"Area A", "Path A", "E2_ECE_300_CL_001", "nsu=http://Tankhouse_Data_2;i=2880", "i=10", "Float"},
		{"Area A", "Path A", "APM_AUTO", "nsu=http://Tankhouse_Data_2;i=4431", "i=1", "Boolean"},
		{"Area A", "Path A", "STACK_COUNT", "nsu=http://Tankhouse_Data_2;i=4448", "i=4", "Int16"},
		{"Area A", "Path A", "BAD", "not-a-node", "i=10", "Float"},
	}
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	res, err := Parse(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tags) != 3 {
		t.Fatalf("tags=%d errors=%v", len(res.Tags), res.Errors)
	}
	if res.Tags[0].DataType != "float64" || res.Tags[1].DataType != "bool" || res.Tags[2].DataType != "int64" {
		t.Fatalf("types %#v", res.Tags)
	}
	if len(res.Errors) == 0 {
		t.Fatal("expected error for bad node id")
	}
}
