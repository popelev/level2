package projectxlsx

import (
	"bytes"
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestWriteParseRoundTrip(t *testing.T) {
	devs := []core.Device{{
		ID: "s7", Endpoint: "opc.tcp://10.0.0.1:4840", Username: "u", Security: "None",
		Tags: []core.Tag{{
			ID: "t1", NodeID: "ns=4;i=4208", Path: "Area/Path", DataType: core.ValueFloat64,
			Enabled: true, IntervalMs: 1000,
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
	if p.Legacy || len(p.Devices) != 1 {
		t.Fatalf("%#v", p)
	}
	if p.Devices[0].ID != "s7" || len(p.Devices[0].Tags) != 1 {
		t.Fatalf("%#v", p.Devices[0])
	}
	if p.Devices[0].Tags[0].Path != "Area/Path" {
		t.Fatalf("path=%q", p.Devices[0].Tags[0].Path)
	}
}
