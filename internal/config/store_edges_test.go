package config

import (
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestSetDeviceTags(t *testing.T) {
	s := testStore(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None",
		Tags: []core.Tag{{ID: "old", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000}},
	})
	genBefore := s.Gen()
	err := s.SetDeviceTags("plc", []core.Tag{
		{ID: "a", NodeID: "ns=4;i=2", DataType: core.ValueBool, Enabled: true, IntervalMs: 250},
		{ID: "b", NodeID: "ns=4;i=3", DataType: core.ValueString, Enabled: false, IntervalMs: 500},
	})
	if err != nil {
		t.Fatal(err)
	}
	tags, err := s.DeviceTags("plc")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0].ID != "a" || tags[1].ID != "b" {
		t.Fatalf("%#v", tags)
	}
	if s.Gen() <= genBefore {
		t.Fatalf("gen did not bump: before=%d after=%d", genBefore, s.Gen())
	}
	if err := s.SetDeviceTags("missing", nil); err == nil {
		t.Fatal("expected missing device error")
	}
}

func TestSetCapacityPolicy(t *testing.T) {
	s := testStore(t)
	genBefore := s.Gen()
	if err := s.SetCapacityPolicy(75, FullPolicyDropOldest); err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	if snap.Database.CapacityPercent != 75 || snap.Database.FullPolicy != FullPolicyDropOldest {
		t.Fatalf("%+v", snap.Database)
	}
	if s.Gen() <= genBefore {
		t.Fatalf("gen=%d", s.Gen())
	}
	if err := s.SetCapacityPolicy(0, FullPolicyStop); err == nil {
		t.Fatal("expected invalid percent")
	}
	if err := s.SetCapacityPolicy(50, "nope"); err == nil {
		t.Fatal("expected invalid policy")
	}
}

func TestUpsertDevice_ValidationErrors(t *testing.T) {
	s := testStore(t)
	if err := s.UpsertDevice(core.Device{Endpoint: "opc.tcp://x"}); err == nil {
		t.Fatal("empty id")
	}
	if err := s.UpsertDevice(core.Device{ID: "x"}); err == nil {
		t.Fatal("empty endpoint")
	}
	if err := s.UpsertDevice(core.Device{ID: "new", Endpoint: "opc.tcp://z:4840"}); err != nil {
		t.Fatal(err)
	}
	devs := s.Devices()
	if len(devs) != 1 || devs[0].ID != "new" || devs[0].Security != "None" {
		t.Fatalf("%#v", devs)
	}
	if got := core.NormalizePollConcurrency(devs[0].PollConcurrency); got != devs[0].PollConcurrency {
		t.Fatalf("poll concurrency not normalized: %d", devs[0].PollConcurrency)
	}
}

func TestDeviceTags_Missing(t *testing.T) {
	s := testStore(t)
	if _, err := s.DeviceTags("nope"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := s.ClearDeviceTags("nope"); err == nil {
		t.Fatal("expected error")
	}
	if err := s.DeleteTag("nope", "t"); err == nil {
		t.Fatal("expected error")
	}
	if err := s.DeleteDevice("nope"); err == nil {
		t.Fatal("expected error")
	}
}
