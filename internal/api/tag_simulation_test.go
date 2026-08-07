package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/popelev/level2/internal/config"
)

func TestTagSimulation_GetPut(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	yaml := `
listen: ":8080"
database:
  url: postgres://u:p@localhost:5432/db
devices:
  - id: plc
    endpoint: opc.tcp://plc:4840
    tags:
      - id: t1
        node_id: ns=4;i=1
        datatype: float64
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	store := config.NewStore(path, f)
	s := &Server{
		Cfg:                 store,
		TagSimulationActive: func() bool { return false },
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tag-simulation", nil))
	if rr.Code != 200 {
		t.Fatalf("get %d %s", rr.Code, rr.Body.String())
	}
	var got tagSimulationDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Enabled || got.Active || got.RestartRequired {
		t.Fatalf("default off: %#v", got)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/tag-simulation",
		strings.NewReader(`{"enabled":true}`))
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req)
	if rr2.Code != 200 {
		t.Fatalf("put %d %s", rr2.Code, rr2.Body.String())
	}
	var after tagSimulationDTO
	if err := json.Unmarshal(rr2.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if !after.Enabled || after.Active || !after.RestartRequired {
		t.Fatalf("want enabled+restart: %#v", after)
	}
	if !store.TagSimulation() {
		t.Fatal("store not persisted")
	}
}
