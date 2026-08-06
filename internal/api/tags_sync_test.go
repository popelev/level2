package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
)

// Sync must overwrite every selected tag's datatype (not only empty/invalid),
// even when PUT/bulk would keep an explicit float64.
func TestHandleSyncTags_OverwritesWrongDataTypes(t *testing.T) {
	dir := t.TempDir()
	cfg := config.NewStore(filepath.Join(dir, "config.yaml"), &config.File{
		Listen:   ":0",
		SpoolDir: dir,
		UIDir:    dir,
		Devices: []core.Device{{
			ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None",
			Tags: []core.Tag{
				{ID: "objects_serverinterfaces_tankhouse_data_3_anodes_time", NodeID: "ns=6;i=166", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
				{ID: "tank_sunit", NodeID: "ns=5;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
				{ID: "bEnable", NodeID: "ns=4;i=2", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
				{ID: "rValueOut", NodeID: "ns=4;i=3", DataType: core.ValueString, Enabled: true, IntervalMs: 1000},
				{ID: "keep_me", NodeID: "ns=4;i=9", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
			},
		}},
	})

	s := &Server{Cfg: cfg} // no DevHub → Guess path (same overwrite contract as Sync)
	mux := http.NewServeMux()
	s.mountTagBulk(mux)

	body := []byte(`{"tag_ids":["objects_serverinterfaces_tankhouse_data_3_anodes_time","tank_sunit","bEnable","rValueOut"]}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/sync", bytes.NewReader(body))
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if int(resp["total"].(float64)) != 4 {
		t.Fatalf("total=%v", resp["total"])
	}
	if int(resp["updated"].(float64)) < 3 {
		t.Fatalf("updated=%v want ≥3", resp["updated"])
	}

	tags, err := cfg.DeviceTags("plc")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]core.Tag{}
	for _, tg := range tags {
		byID[tg.ID] = tg
	}
	if byID["objects_serverinterfaces_tankhouse_data_3_anodes_time"].DataType != core.ValueDateTime {
		t.Fatalf("Time: %#v", byID["objects_serverinterfaces_tankhouse_data_3_anodes_time"])
	}
	if byID["tank_sunit"].DataType != core.ValueString {
		t.Fatalf("sunit: %#v", byID["tank_sunit"])
	}
	if byID["bEnable"].DataType != core.ValueBool {
		t.Fatalf("bool: %#v", byID["bEnable"])
	}
	if byID["rValueOut"].DataType != core.ValueFloat64 {
		t.Fatalf("rValueOut: %#v", byID["rValueOut"])
	}
	if byID["keep_me"].DataType != core.ValueFloat64 {
		t.Fatalf("keep_me should stay float64: %#v", byID["keep_me"])
	}
}

func TestResolveTagDataType_DoesNotOverwriteOnPutPath(t *testing.T) {
	s := &Server{}
	tg := core.Tag{
		ID: "objects_serverinterfaces_tankhouse_data_3_anodes_time",
		NodeID: "ns=6;i=166", DataType: core.ValueFloat64,
	}
	s.resolveTagDataType(context.Background(), "plc", &tg)
	if tg.DataType != core.ValueFloat64 {
		t.Fatalf("PUT path must keep explicit datatype, got %q", tg.DataType)
	}
}
