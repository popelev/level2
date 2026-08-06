package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestApplyDeviceEnvOverlay(t *testing.T) {
	ApplyDeviceEnvOverlay(nil) // no panic

	t.Setenv("PLC_OPC_ENDPOINT", "opc.tcp://from-env:4840")
	t.Setenv("OPC_UA_USERNAME", "u1")
	t.Setenv("OPC_UA_PASSWORD", "p1")
	d := &core.Device{ID: "x", Endpoint: "opc.tcp://old"}
	ApplyDeviceEnvOverlay(d)
	if d.Endpoint != "opc.tcp://from-env:4840" || d.Username != "u1" || d.Password != "p1" {
		t.Fatalf("%#v", d)
	}
}

func TestLoad_YAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	yaml := `
listen: ":9090"
database:
  url: postgres://u:p@localhost:5432/db
  capacity_percent: 80
  full_policy: drop_oldest
spool_dir: /tmp/spool
ui_dir: /tmp/ui
devices:
  - id: plc
    endpoint: opc.tcp://plc:4840
    security: None
    poll_concurrency: 2
    tags:
      - id: t1
        node_id: ns=4;i=1
        datatype: float
        enabled: true
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Listen != ":9090" || f.Database.CapacityPercent != 80 || f.Database.FullPolicy != FullPolicyDropOldest {
		t.Fatalf("%+v", f)
	}
	if len(f.Devices) != 1 || f.Devices[0].Tags[0].DataType != core.ValueFloat64 {
		t.Fatalf("%#v", f.Devices)
	}
	if f.Devices[0].PollConcurrency != 2 {
		t.Fatalf("poll=%d", f.Devices[0].PollConcurrency)
	}

	t.Setenv("DATABASE_URL", "postgres://env:pw@db:5432/envdb")
	t.Setenv("SPOOL_DIR", "/env/spool")
	t.Setenv("UI_DIR", "/env/ui")
	t.Setenv("LEVEL2_DB_CAPACITY_PERCENT", "70")
	t.Setenv("LEVEL2_DB_FULL_POLICY", "stop")
	f2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if f2.Database.URL != "postgres://env:pw@db:5432/envdb" {
		t.Fatalf("url=%q", f2.Database.URL)
	}
	if f2.SpoolDir != "/env/spool" || f2.UIDir != "/env/ui" {
		t.Fatalf("dirs %#v", f2)
	}
	if f2.Database.CapacityPercent != 70 || f2.Database.FullPolicy != FullPolicyStop {
		t.Fatalf("capacity %+v", f2.Database)
	}
}

func TestLoad_ValidationErrors(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name string
		yaml string
	}{
		{"no db", "devices: []\n"},
		{"bad capacity", "database:\n  url: postgres://u:p@h/db\n  capacity_percent: 0\n"},
		{"bad policy", "database:\n  url: postgres://u:p@h/db\n  full_policy: nope\n"},
		{"no endpoint", "database:\n  url: postgres://u:p@h/db\ndevices:\n  - id: x\n"},
		{"bad tag node", "database:\n  url: postgres://u:p@h/db\ndevices:\n  - id: x\n    endpoint: opc.tcp://x\n    tags:\n      - id: t\n        node_id: bad\n"},
		{"bad datatype", "database:\n  url: postgres://u:p@h/db\ndevices:\n  - id: x\n    endpoint: opc.tcp://x\n    tags:\n      - id: t\n        node_id: ns=1;i=1\n        datatype: struct\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o644); err != nil {
				t.Fatal(err)
			}
			// capacity_percent: 0 is normalized to 90 — adjust that case
			if tc.name == "bad capacity" {
				_ = os.WriteFile(path, []byte("database:\n  url: postgres://u:p@h/db\n  capacity_percent: 101\n"), 0o644)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestClearDeviceTagsAndAllTags(t *testing.T) {
	s := testStore(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x", Security: "None",
		Tags: []core.Tag{
			{ID: "a", NodeID: "ns=1;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
			{ID: "b", NodeID: "ns=1;i=2", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
		},
	})
	n, err := s.ClearDeviceTags("plc")
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(s.AllTags()) != 0 {
		t.Fatal(s.AllTags())
	}
	if s.Gen() < 2 {
		t.Fatalf("gen=%d", s.Gen())
	}
}
