package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/popelev/level2/internal/core"
)

func testStore(t *testing.T, devices ...core.Device) *Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	f := &File{
		Listen:   ":8080",
		Database: Database{URL: "postgres://u:p@localhost/db", CapacityPercent: 90, FullPolicy: FullPolicyStop},
		SpoolDir: dir,
		UIDir:    dir,
		Devices:  append([]core.Device(nil), devices...),
	}
	s := NewStore(path, f)
	// Persist initial snapshot without secrets in YAML.
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMergeTags_AddUpdateReplace(t *testing.T) {
	s := testStore(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None",
		Tags: []core.Tag{{ID: "a", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 500}},
	})

	added, updated, err := s.MergeTags("plc", []core.Tag{
		{ID: "a", NodeID: "ns=4;i=2", DataType: core.ValueFloat64, Enabled: true}, // update + default interval
		{ID: "b", NodeID: "ns=4;i=3", DataType: core.ValueBool, Enabled: true, IntervalMs: 200},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 || updated != 1 {
		t.Fatalf("added=%d updated=%d", added, updated)
	}
	tags, err := s.DeviceTags("plc")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 {
		t.Fatalf("len=%d", len(tags))
	}
	byID := map[string]core.Tag{}
	for _, tg := range tags {
		byID[tg.ID] = tg
	}
	if byID["a"].NodeID != "ns=4;i=2" || byID["a"].IntervalMs != 1000 {
		t.Fatalf("updated tag a: %#v", byID["a"])
	}
	if byID["b"].IntervalMs != 200 {
		t.Fatalf("new tag b: %#v", byID["b"])
	}

	added, updated, err = s.MergeTags("plc", []core.Tag{
		{ID: "c", NodeID: "ns=4;i=9", DataType: core.ValueString, Enabled: true, IntervalMs: 100},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 || updated != 0 {
		t.Fatalf("replace: added=%d updated=%d", added, updated)
	}
	tags, _ = s.DeviceTags("plc")
	if len(tags) != 1 || tags[0].ID != "c" {
		t.Fatalf("replace tags=%#v", tags)
	}
}

func TestMergeTags_UnknownDevice(t *testing.T) {
	s := testStore(t)
	_, _, err := s.MergeTags("missing", nil, false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestApplyProject_MergeKeepsPasswordAndEmptyTags(t *testing.T) {
	s := testStore(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://old:4840", Password: "secret", Security: "None",
		Tags: []core.Tag{{ID: "keep", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000}},
	})

	err := s.ApplyProject([]core.Device{{
		ID: "plc", Endpoint: "opc.tcp://new:4840", Password: "from-xlsx-ignored", Security: "Sign",
		Tags: nil, // empty → keep previous tags on merge
	}, {
		ID: "other", Endpoint: "opc.tcp://y:4840", // new device
	}}, false)
	if err != nil {
		t.Fatal(err)
	}
	devs := s.Devices()
	if len(devs) != 2 {
		t.Fatalf("len=%d", len(devs))
	}
	var plc core.Device
	for _, d := range devs {
		if d.ID == "plc" {
			plc = d
		}
	}
	if plc.Endpoint != "opc.tcp://new:4840" || plc.Security != "Sign" {
		t.Fatalf("%#v", plc)
	}
	if plc.Password != "secret" {
		t.Fatalf("password must stay in memory store, got %q", plc.Password)
	}
	if len(plc.Tags) != 1 || plc.Tags[0].ID != "keep" {
		t.Fatalf("tags should be preserved on merge with empty import: %#v", plc.Tags)
	}

	// YAML on disk must not contain the password.
	raw, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") {
		t.Fatalf("password leaked into YAML:\n%s", raw)
	}
}

func TestApplyProject_Replace(t *testing.T) {
	s := testStore(t,
		core.Device{ID: "old", Endpoint: "opc.tcp://a:4840", Password: "p1", Security: "None"},
		core.Device{ID: "keep", Endpoint: "opc.tcp://b:4840", Password: "p2", Security: "None"},
	)
	err := s.ApplyProject([]core.Device{{
		ID: "keep", Endpoint: "opc.tcp://b2:4840", Security: "None",
		Tags: []core.Tag{{ID: "t", NodeID: "ns=2;i=1", DataType: core.ValueInt64, Enabled: true, IntervalMs: 1000}},
	}}, true)
	if err != nil {
		t.Fatal(err)
	}
	devs := s.Devices()
	if len(devs) != 1 || devs[0].ID != "keep" {
		t.Fatalf("%#v", devs)
	}
	if devs[0].Password != "p2" {
		t.Fatalf("password=%q", devs[0].Password)
	}
	if len(devs[0].Tags) != 1 {
		t.Fatalf("tags=%#v", devs[0].Tags)
	}
}

func TestUpsertTag_ValidationAndUpdate(t *testing.T) {
	s := testStore(t, core.Device{ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None"})

	if err := s.UpsertTag("plc", core.Tag{ID: "", NodeID: "ns=4;i=1"}); err == nil {
		t.Fatal("empty id")
	}
	if err := s.UpsertTag("plc", core.Tag{ID: "t", NodeID: "bad"}); err == nil {
		t.Fatal("bad node id")
	}
	if err := s.UpsertTag("plc", core.Tag{ID: "t", NodeID: "ns=4;i=1", DataType: "nope"}); err == nil {
		t.Fatal("bad datatype")
	}

	err := s.UpsertTag("plc", core.Tag{ID: "t1", NodeID: "ns=4;i=1", DataType: "float", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	tags, _ := s.DeviceTags("plc")
	if len(tags) != 1 || tags[0].DataType != core.ValueFloat64 || tags[0].IntervalMs != 1000 {
		t.Fatalf("%#v", tags[0])
	}

	err = s.UpsertTag("plc", core.Tag{ID: "t1", NodeID: "ns=4;i=99", DataType: core.ValueBool, Enabled: false, IntervalMs: 250})
	if err != nil {
		t.Fatal(err)
	}
	tags, _ = s.DeviceTags("plc")
	if len(tags) != 1 || tags[0].NodeID != "ns=4;i=99" || tags[0].DataType != core.ValueBool {
		t.Fatalf("%#v", tags[0])
	}
}

func TestUpsertDevice_PreservesPasswordAndTags(t *testing.T) {
	s := testStore(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x:4840", Password: "keep-me", Security: "None",
		Tags: []core.Tag{{ID: "t", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000}},
	})
	err := s.UpsertDevice(core.Device{ID: "plc", Endpoint: "opc.tcp://y:4840", Security: "None"})
	if err != nil {
		t.Fatal(err)
	}
	d := s.Devices()[0]
	if d.Endpoint != "opc.tcp://y:4840" || d.Password != "keep-me" || len(d.Tags) != 1 {
		t.Fatalf("%#v", d)
	}
}

func TestDeleteTagAndDevice(t *testing.T) {
	s := testStore(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None",
		Tags: []core.Tag{
			{ID: "a", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
			{ID: "b", NodeID: "ns=4;i=2", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
		},
	})
	if err := s.DeleteTag("plc", "a"); err != nil {
		t.Fatal(err)
	}
	tags, _ := s.DeviceTags("plc")
	if len(tags) != 1 || tags[0].ID != "b" {
		t.Fatalf("%#v", tags)
	}
	if err := s.DeleteDevice("plc"); err != nil {
		t.Fatal(err)
	}
	if len(s.Devices()) != 0 {
		t.Fatal("expected empty")
	}
}

func TestUpsertDevice_NormalizesPollConcurrency(t *testing.T) {
	s := testStore(t, core.Device{ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None"})
	if err := s.UpsertDevice(core.Device{ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None", PollConcurrency: 0}); err != nil {
		t.Fatal(err)
	}
	if got := s.Devices()[0].PollConcurrency; got != core.DefaultPollConcurrency {
		t.Fatalf("default: got %d want %d", got, core.DefaultPollConcurrency)
	}
	if err := s.UpsertDevice(core.Device{ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None", PollConcurrency: 99}); err != nil {
		t.Fatal(err)
	}
	if got := s.Devices()[0].PollConcurrency; got != core.MaxPollConcurrency {
		t.Fatalf("clamp: got %d want %d", got, core.MaxPollConcurrency)
	}
	if err := s.UpsertDevice(core.Device{ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None", PollConcurrency: 2}); err != nil {
		t.Fatal(err)
	}
	if got := s.Devices()[0].PollConcurrency; got != 2 {
		t.Fatalf("keep: got %d", got)
	}
}

func TestSetTagsSimulate(t *testing.T) {
	s := testStore(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None",
		Tags: []core.Tag{
			{ID: "a", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
			{ID: "b", NodeID: "ns=4;i=2", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
		},
	})
	if s.CountSimulatedTags() != 0 {
		t.Fatal("default false")
	}
	n, err := s.SetTagsSimulate("plc", []string{"a"}, true)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if s.CountSimulatedTags() != 1 {
		t.Fatal(s.CountSimulatedTags())
	}
	n, err = s.SetTagsSimulate("plc", nil, true)
	if err != nil || n != 2 {
		t.Fatalf("all n=%d err=%v", n, err)
	}
	tags, _ := s.DeviceTags("plc")
	for _, tg := range tags {
		if !tg.Simulate {
			t.Fatalf("%s not simulated", tg.ID)
		}
	}
	n, err = s.SetTagsSimulate("plc", []string{"missing"}, false)
	if err == nil || n != 0 {
		t.Fatalf("want error for missing, n=%d err=%v", n, err)
	}
}

func TestSetTagsWritable(t *testing.T) {
	s := testStore(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None",
		Tags: []core.Tag{
			{ID: "a", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
			{ID: "b", NodeID: "ns=4;i=2", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
		},
	})
	tags, _ := s.DeviceTags("plc")
	for _, tg := range tags {
		if tg.Writable {
			t.Fatalf("%s default writable want false", tg.ID)
		}
	}
	n, err := s.SetTagsWritable("plc", []string{"a"}, true)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	tags, _ = s.DeviceTags("plc")
	byID := map[string]core.Tag{}
	for _, tg := range tags {
		byID[tg.ID] = tg
	}
	if !byID["a"].Writable || byID["b"].Writable {
		t.Fatalf("a=%v b=%v", byID["a"].Writable, byID["b"].Writable)
	}
	n, err = s.SetTagsWritable("plc", nil, true)
	if err != nil || n != 2 {
		t.Fatalf("all n=%d err=%v", n, err)
	}
	n, err = s.SetTagsWritable("plc", []string{"missing"}, false)
	if err == nil || n != 0 {
		t.Fatalf("want error for missing, n=%d err=%v", n, err)
	}
}
