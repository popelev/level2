package config

import (
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestStoreFlags_NilAndRoundTrip(t *testing.T) {
	var nilStore *Store
	if nilStore.OPCWriteEnabled() || nilStore.TagSimulation() || nilStore.APIToken() != "" {
		t.Fatal("nil store should return zero values")
	}
	if err := nilStore.SetTagSimulation(true); err == nil {
		t.Fatal("expected nil store error")
	}

	empty := NewStore(t.TempDir()+"/x.yaml", nil)
	if empty.OPCWriteEnabled() || empty.TagSimulation() || empty.APIToken() != "" {
		t.Fatal("nil file should return zero values")
	}
	if err := empty.SetTagSimulation(true); err == nil {
		t.Fatal("expected nil file error")
	}

	s := testStore(t, core.Device{ID: "plc", Endpoint: "opc.tcp://x", Security: "None"})
	if s.OPCWriteEnabled() || s.TagSimulation() || s.APIToken() != "" {
		t.Fatalf("defaults: write=%v sim=%v token=%q", s.OPCWriteEnabled(), s.TagSimulation(), s.APIToken())
	}

	s.mu.Lock()
	s.file.OPCWriteEnabled = true
	s.file.APIToken = "secret"
	s.mu.Unlock()
	if !s.OPCWriteEnabled() || s.APIToken() != "secret" {
		t.Fatalf("write=%v token=%q", s.OPCWriteEnabled(), s.APIToken())
	}

	if err := s.SetTagSimulation(true); err != nil {
		t.Fatal(err)
	}
	if !s.TagSimulation() {
		t.Fatal("expected tag simulation on")
	}
	if err := s.SetTagSimulation(false); err != nil {
		t.Fatal(err)
	}
	if s.TagSimulation() {
		t.Fatal("expected tag simulation off")
	}
}

func TestParseEnvBool(t *testing.T) {
	trues := []string{"1", "true", "TRUE", " yes ", "on"}
	for _, v := range trues {
		got, err := parseEnvBool(v)
		if err != nil || !got {
			t.Fatalf("%q: got=%v err=%v", v, got, err)
		}
	}
	falses := []string{"0", "false", "NO", " off "}
	for _, v := range falses {
		got, err := parseEnvBool(v)
		if err != nil || got {
			t.Fatalf("%q: got=%v err=%v", v, got, err)
		}
	}
	if _, err := parseEnvBool("maybe"); err == nil {
		t.Fatal("expected error")
	}
}
