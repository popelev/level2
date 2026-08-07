package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestSetTagSimulation_PersistsAcrossReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	s := NewStore(path, &File{
		Listen: ":0", SpoolDir: dir, UIDir: dir,
		Database: Database{URL: "postgres://u:p@localhost/db", CapacityPercent: 90, FullPolicy: FullPolicyStop},
		Devices:  []core.Device{{ID: "plc", Endpoint: "opc.tcp://x", Security: "None"}},
	})
	if err := s.SetTagSimulation(true); err != nil {
		t.Fatal(err)
	}
	if !s.TagSimulation() {
		t.Fatal("in-memory")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "tag_simulation: true") {
		t.Fatalf("yaml missing flag: %s", raw)
	}

	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !f.TagSimulation {
		t.Fatal("reloaded tag_simulation")
	}

	s2 := NewStore(path, f)
	if !s2.TagSimulation() || s2.OPCWriteEnabled() || s2.APIToken() != "" {
		t.Fatalf("store2 write=%v sim=%v token=%q", s2.OPCWriteEnabled(), s2.TagSimulation(), s2.APIToken())
	}
}

func TestParseEnvBool_EmptyAndWhitespace(t *testing.T) {
	if _, err := parseEnvBool(""); err == nil {
		t.Fatal("empty")
	}
	if _, err := parseEnvBool("   "); err == nil {
		t.Fatal("whitespace")
	}
	got, err := parseEnvBool(" Yes ")
	if err != nil || !got {
		t.Fatalf("Yes: %v %v", got, err)
	}
}
