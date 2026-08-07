package projectxlsx

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/popelev/level2/internal/core"
	"github.com/xuri/excelize/v2"
)

func TestParse_InvalidXLSX(t *testing.T) {
	if _, err := Parse(strings.NewReader("not-an-xlsx")); err == nil {
		t.Fatal("expected open error")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read fail") }

func TestParse_ReadError(t *testing.T) {
	if _, err := Parse(errReader{}); err == nil {
		t.Fatal("expected read error")
	}
}

func TestParse_LegacyParseError(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellValue(sheet, "A1", "Nope")
	_ = f.SetCellValue(sheet, "A2", "x")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(&buf); err == nil {
		t.Fatal("expected legacy parse error")
	}
}

func TestParse_LegacyPlantSheet(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	headers := []string{"Area", "Path", "Signal", "MeasurePoint NodeId", "DataType", "DataType Name"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	_ = f.SetCellValue(sheet, "A2", "Area A")
	_ = f.SetCellValue(sheet, "B2", "Path A")
	_ = f.SetCellValue(sheet, "C2", "SIG_1")
	_ = f.SetCellValue(sheet, "D2", "ns=4;i=100")
	_ = f.SetCellValue(sheet, "E2", "i=10")
	_ = f.SetCellValue(sheet, "F2", "Float")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	p, err := Parse(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Legacy || len(p.LegacyTags) != 1 || p.LegacyTags[0].ID != "SIG_1" {
		t.Fatalf("%#v", p)
	}
}

func TestWriteParse_WritableSimulateAndDefaults(t *testing.T) {
	devs := []core.Device{{
		ID: "plc", Endpoint: "opc.tcp://1.2.3.4:4840", Username: "op", Security: "",
		Tags: []core.Tag{{
			ID: "t1", NodeID: "ns=4;i=1", Path: "P", DataType: core.ValueBool,
			Enabled: true, IntervalMs: 250, Writable: true, Simulate: true,
		}},
	}}
	b, err := Write(devs)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Devices) != 1 || len(p.Devices[0].Tags) != 1 {
		t.Fatalf("%#v", p)
	}
	tag := p.Devices[0].Tags[0]
	if !tag.Writable || !tag.Simulate || tag.IntervalMs != 250 || tag.DataType != core.ValueBool {
		t.Fatalf("%#v", tag)
	}
}

func TestParseProject_ErrorPaths(t *testing.T) {
	f := excelize.NewFile()
	_ = f.SetSheetName(f.GetSheetName(0), SheetServers)
	for i, h := range []string{"id", "endpoint", "username", "security"} {
		_ = f.SetCellValue(SheetServers, cell(i+1, 1), h)
	}
	// empty id skipped
	_ = f.SetCellValue(SheetServers, cell(1, 2), "")
	_ = f.SetCellValue(SheetServers, cell(2, 2), "opc.tcp://x")
	// empty endpoint -> error + skip
	_ = f.SetCellValue(SheetServers, cell(1, 3), "badep")
	_ = f.SetCellValue(SheetServers, cell(2, 3), "")
	// ok server, empty security -> None
	_ = f.SetCellValue(SheetServers, cell(1, 4), "ok")
	_ = f.SetCellValue(SheetServers, cell(2, 4), "opc.tcp://ok:4840")
	_ = f.SetCellValue(SheetServers, cell(3, 4), "u")

	_, _ = f.NewSheet(SheetTags)
	for i, h := range []string{"device_id", "id", "path", "node_id", "datatype", "enabled", "interval_ms", "writable", "simulate"} {
		_ = f.SetCellValue(SheetTags, cell(i+1, 1), h)
	}
	row := 2
	setTag := func(dev, id, node, dt, en, iv, wr, sim string) {
		_ = f.SetCellValue(SheetTags, cell(1, row), dev)
		_ = f.SetCellValue(SheetTags, cell(2, row), id)
		_ = f.SetCellValue(SheetTags, cell(4, row), node)
		_ = f.SetCellValue(SheetTags, cell(5, row), dt)
		_ = f.SetCellValue(SheetTags, cell(6, row), en)
		_ = f.SetCellValue(SheetTags, cell(7, row), iv)
		_ = f.SetCellValue(SheetTags, cell(8, row), wr)
		_ = f.SetCellValue(SheetTags, cell(9, row), sim)
		row++
	}
	setTag("", "", "", "", "", "", "", "") // skip: empty id+node
	setTag("", "t_miss_dev", "ns=1;i=1", "", "", "", "", "")
	setTag("ok", "", "ns=1;i=2", "", "", "", "", "")
	setTag("ok", "badnode", "not-a-node", "", "", "", "", "")
	setTag("ok", "baddt", "ns=1;i=3", "struct", "", "", "", "")
	setTag("missing", "orphan", "ns=1;i=4", "", "yes", "0", "1", "on")
	setTag("ok", "good", "ns=1;i=5", "", "", "", "no", "off") // empty dt -> float64, interval default

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	p, err := Parse(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Errors) < 5 {
		t.Fatalf("errors=%v", p.Errors)
	}
	var okDev, orphanDev *core.Device
	for i := range p.Devices {
		d := &p.Devices[i]
		switch d.ID {
		case "ok":
			okDev = d
		case "missing":
			orphanDev = d
		case "badep":
			t.Fatal("empty-endpoint server must be skipped")
		}
	}
	if okDev == nil || okDev.Security != "None" {
		t.Fatalf("ok device %#v", okDev)
	}
	var good *core.Tag
	for i := range okDev.Tags {
		if okDev.Tags[i].ID == "good" {
			good = &okDev.Tags[i]
		}
	}
	if good == nil || good.DataType != core.ValueFloat64 || good.IntervalMs != 1000 || good.Writable || good.Simulate {
		t.Fatalf("good tag %#v", good)
	}
	if orphanDev == nil || orphanDev.Endpoint != "opc.tcp://unknown" || len(orphanDev.Tags) != 1 {
		t.Fatalf("orphan %#v", orphanDev)
	}
	if !orphanDev.Tags[0].Writable || !orphanDev.Tags[0].Simulate {
		t.Fatalf("orphan flags %#v", orphanDev.Tags[0])
	}
}

func TestParseProject_EmptyWorkbook(t *testing.T) {
	f := excelize.NewFile()
	_ = f.SetSheetName(f.GetSheetName(0), SheetServers)
	_ = f.SetCellValue(SheetServers, "A1", "id")
	_ = f.SetCellValue(SheetServers, "B1", "endpoint")
	_, _ = f.NewSheet(SheetTags)
	_ = f.SetCellValue(SheetTags, "A1", "device_id")
	_ = f.SetCellValue(SheetTags, "B1", "id")
	_ = f.SetCellValue(SheetTags, "C1", "node_id")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	_, err := Parse(&buf)
	if err == nil || !strings.Contains(err.Error(), "no servers/tags") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseProject_TagsOnlyCreatesPlaceholder(t *testing.T) {
	f := excelize.NewFile()
	_ = f.SetSheetName(f.GetSheetName(0), SheetTags)
	for i, h := range []string{"deviceid", "id", "nodeid", "datatype", "intervalms"} {
		_ = f.SetCellValue(SheetTags, cell(i+1, 1), h)
	}
	_ = f.SetCellValue(SheetTags, cell(1, 2), "solo")
	_ = f.SetCellValue(SheetTags, cell(2, 2), "t1")
	_ = f.SetCellValue(SheetTags, cell(3, 2), "ns=2;i=9")
	_ = f.SetCellValue(SheetTags, cell(4, 2), "bool")
	_ = f.SetCellValue(SheetTags, cell(5, 2), "-5")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	p, err := Parse(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Devices) != 1 || p.Devices[0].ID != "solo" {
		t.Fatalf("%#v", p.Devices)
	}
	if len(p.Errors) == 0 {
		t.Fatal("expected placeholder warning")
	}
	tag := p.Devices[0].Tags[0]
	if tag.DataType != core.ValueBool || tag.IntervalMs != 1000 {
		t.Fatalf("%#v", tag)
	}
}

func TestParse_CaseInsensitiveSheetNames(t *testing.T) {
	f := excelize.NewFile()
	_ = f.SetSheetName(f.GetSheetName(0), "servers")
	_ = f.SetCellValue("servers", "A1", "id")
	_ = f.SetCellValue("servers", "B1", "endpoint")
	_ = f.SetCellValue("servers", "A2", "s1")
	_ = f.SetCellValue("servers", "B2", "opc.tcp://case:4840")
	_, _ = f.NewSheet("tags")
	_ = f.SetCellValue("tags", "A1", "device_id")
	_ = f.SetCellValue("tags", "B1", "id")
	_ = f.SetCellValue("tags", "C1", "node_id")
	_ = f.SetCellValue("tags", "A2", "s1")
	_ = f.SetCellValue("tags", "B2", "t1")
	_ = f.SetCellValue("tags", "C2", "ns=1;i=1")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	p, err := Parse(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Devices) != 1 || len(p.Devices[0].Tags) != 1 {
		t.Fatalf("%#v", p)
	}
}

func TestFindSheetAndSplitKey(t *testing.T) {
	f := excelize.NewFile()
	_ = f.SetSheetName(f.GetSheetName(0), "Other")
	if findSheet(f, SheetServers) != "" {
		t.Fatal("expected miss")
	}
	if sheetNamed([]string{"a", "Tags"}, SheetTags) != true || sheetNamed([]string{"a"}, SheetTags) {
		t.Fatal("sheetNamed")
	}
	dev, tid := splitKey("d\x00t")
	if dev != "d" || tid != "t" {
		t.Fatalf("%q %q", dev, tid)
	}
	if d, id := splitKey("plain"); d != "" || id != "plain" {
		t.Fatalf("%q %q", d, id)
	}
}

func TestMapHeaders_Aliases(t *testing.T) {
	m := mapHeaders([]string{"DeviceId", "Node Id", "IntervalMs", "Simulate", "Writable"})
	if m["device_id"] < 0 || m["node_id"] < 0 || m["interval_ms"] < 0 || m["simulate"] < 0 || m["writable"] < 0 {
		t.Fatalf("%#v", m)
	}
}
