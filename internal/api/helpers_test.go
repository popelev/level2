package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	devruntime "github.com/popelev/level2/internal/runtime"
	"github.com/popelev/level2/internal/store"
)

func TestSampleDTOAndInferSampleType(t *testing.T) {
	n := 3.5
	txt := "ok"
	b := true
	now := time.Unix(100, 0).UTC()

	cases := []struct {
		name string
		s    core.Sample
		want string
	}{
		{"bool", core.Sample{ValueBool: &b}, string(core.ValueBool)},
		{"text", core.Sample{ValueText: &txt}, string(core.ValueString)},
		{"num", core.Sample{ValueNum: &n}, string(core.ValueFloat64)},
		{"empty", core.Sample{}, ""},
	}
	for _, tc := range cases {
		if got := inferSampleType(tc.s); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}

	dto := sampleDTO(core.Sample{
		Time: now, TagID: "t", ValueNum: &n, Quality: core.QualityBad,
	})
	if dto.TagID != "t" || dto.Quality != int(core.QualityBad) || dto.ValueNum == nil || *dto.ValueNum != 3.5 {
		t.Fatalf("%#v", dto)
	}
}

func TestHandleDevicesAndHistoryUnavailable(t *testing.T) {
	live := store.NewLive()
	s := &Server{
		Live: live,
		Devices: func() []core.Device {
			return []core.Device{{ID: "d1", Endpoint: "opc.tcp://x", Tags: []core.Tag{{ID: "a"}}}}
		},
		Hub: NewHub(),
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil))
	if rr.Code != 200 {
		t.Fatalf("devices %d", rr.Code)
	}
	var list []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0]["id"] != "d1" {
		t.Fatalf("%#v", list)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tags/missing/value", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("value missing: %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tags/x/history", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("history: %d", rr.Code)
	}
}

func TestResolveEmptyDataTypesBatch_GuessWhenDisconnected(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := devruntime.NewHub(log, true)
	s := &Server{DevHub: hub}
	tags := []core.Tag{
		{ID: "tank_sunit", NodeID: "ns=5;i=1", DataType: ""},
		{ID: "rValueOut", NodeID: "ns=5;i=2", DataType: "float64"}, // already set
		{ID: "bEnable", NodeID: "ns=5;i=3"},
	}
	// No Upsert → Entry missing → GuessDataType path.
	s.resolveEmptyDataTypesBatch(context.Background(), "dev", tags)
	if tags[0].DataType != core.ValueString {
		t.Fatalf("sunit: got %q", tags[0].DataType)
	}
	if tags[1].DataType != core.ValueFloat64 {
		t.Fatalf("kept float: got %q", tags[1].DataType)
	}
	if tags[2].DataType != core.ValueBool {
		t.Fatalf("benable: got %q", tags[2].DataType)
	}
}

func TestResolveTagDataType_KeepsExplicit(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// DevHub required by resolveTagDataType early-return; normalize does not need a live entry.
	s := &Server{DevHub: devruntime.NewHub(log, true)}
	tg := core.Tag{ID: "x", NodeID: "ns=4;i=1", DataType: "float"}
	s.resolveTagDataType(context.Background(), "d", &tg)
	if tg.DataType != core.ValueFloat64 {
		t.Fatalf("normalized %#v", tg)
	}
	// nil / empty node — no-op
	s.resolveTagDataType(context.Background(), "d", nil)
	empty := core.Tag{ID: "y"}
	s.resolveTagDataType(context.Background(), "d", &empty)
	if empty.DataType != "" {
		t.Fatalf("empty node should stay empty, got %q", empty.DataType)
	}
}

func TestValidateTagFields_DefaultsAndAliases(t *testing.T) {
	cases := []struct {
		in   core.Tag
		want core.ValueType
		ms   int
	}{
		{core.Tag{DataType: "", IntervalMs: -1}, core.ValueFloat64, 1000},
		{core.Tag{DataType: "bool", IntervalMs: 250}, core.ValueBool, 250},
		{core.Tag{DataType: "string", IntervalMs: 10}, core.ValueString, 10},
		{core.Tag{DataType: "datetime", IntervalMs: 1000}, core.ValueDateTime, 1000},
		{core.Tag{DataType: "int64", IntervalMs: 1000}, core.ValueInt64, 1000},
		{core.Tag{DataType: "uint", IntervalMs: 1000}, core.ValueUint, 1000},
	}
	for _, tc := range cases {
		tg := tc.in
		if err := validateTagFields(&tg); err != nil {
			t.Fatalf("%#v: %v", tc.in, err)
		}
		if tg.DataType != tc.want || tg.IntervalMs != tc.ms {
			t.Fatalf("in=%#v got type=%q ms=%d", tc.in, tg.DataType, tg.IntervalMs)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusCreated, map[string]string{"ok": "1"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("code %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("ct=%q", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["ok"] != "1" {
		t.Fatalf("%v", body)
	}
}
