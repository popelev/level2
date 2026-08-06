package projectxlsx

import (
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestCompare_ServersAndTags(t *testing.T) {
	a := []core.Device{{
		ID: "s1", Endpoint: "opc.tcp://a:4840", Username: "u", Security: "None",
		Tags: []core.Tag{{
			ID: "t1", NodeID: "ns=4;i=1", Path: "A/B", DataType: core.ValueFloat64,
			Enabled: true, IntervalMs: 1000,
		}},
	}}
	b := []core.Device{{
		ID: "s1", Endpoint: "opc.tcp://b:4840", Username: "u", Security: "None",
		Tags: []core.Tag{{
			ID: "t1", NodeID: "ns=4;i=2", Path: "A/B", DataType: core.ValueFloat64,
			Enabled: false, IntervalMs: 500,
		}, {
			ID: "t2", NodeID: "ns=4;i=3", DataType: core.ValueBool, Enabled: true, IntervalMs: 1000,
		}},
	}, {
		ID: "s2", Endpoint: "opc.tcp://c:4840", Security: "None",
	}}

	rows := Compare(a, b)
	want := map[string]bool{
		"changed|server|s1|endpoint":  true,
		"changed|tag|s1|t1|node_id":   true,
		"changed|tag|s1|t1|enabled":   true,
		"changed|tag|s1|t1|interval_ms": true,
		"added_in_b|tag|s1|t2":        true,
		"added_in_b|server|s2":        true,
	}
	got := map[string]bool{}
	for _, r := range rows {
		key := r.Status + "|" + r.Kind + "|"
		if r.Kind == "tag" {
			key += r.DeviceID + "|" + r.ID
		} else {
			key += r.ID
		}
		if r.Field != "" {
			key += "|" + r.Field
		}
		got[key] = true
	}
	for k := range want {
		if !got[k] {
			t.Fatalf("missing %s in %#v", k, rows)
		}
	}
}

func TestCompare_RemovedInB(t *testing.T) {
	a := []core.Device{{
		ID: "gone", Endpoint: "opc.tcp://x:4840",
		Tags: []core.Tag{{ID: "t", NodeID: "ns=1;i=1", DataType: core.ValueFloat64}},
	}}
	rows := Compare(a, nil)
	if len(rows) < 2 {
		t.Fatalf("%#v", rows)
	}
	var serverGone, tagGone bool
	for _, r := range rows {
		if r.Status == "removed_in_b" && r.Kind == "server" && r.ID == "gone" {
			serverGone = true
		}
		if r.Status == "removed_in_b" && r.Kind == "tag" && r.ID == "t" {
			tagGone = true
		}
	}
	if !serverGone || !tagGone {
		t.Fatalf("serverGone=%v tagGone=%v rows=%#v", serverGone, tagGone, rows)
	}
}

func TestSummaryAndFormatErr(t *testing.T) {
	servers, tags := Summary([]core.Device{
		{ID: "a", Tags: []core.Tag{{ID: "1"}, {ID: "2"}}},
		{ID: "b", Tags: []core.Tag{{ID: "3"}}},
	})
	if servers != 2 || tags != 3 {
		t.Fatalf("servers=%d tags=%d", servers, tags)
	}
	if FormatErr(nil, 1) != "" {
		t.Fatal("empty")
	}
	if FormatErr([]string{"only"}, 1) != "only" {
		t.Fatal("single")
	}
	got := FormatErr([]string{"a", "b", "c"}, 1)
	if got != "a … (+2)" {
		t.Fatalf("got %q", got)
	}
}

func TestDiffToSheets(t *testing.T) {
	rows := []DiffRow{
		{Status: "changed", Kind: "server", ID: "s1", Field: "endpoint", A: "a", B: "b"},
		{Status: "added_in_b", Kind: "tag", DeviceID: "s1", ID: "t1", Field: "node_id", B: "ns=1;i=1"},
	}
	srv, tags := DiffToSheets(rows)
	if len(srv) != 2 || srv[1][0] != "changed" {
		t.Fatalf("%#v", srv)
	}
	if len(tags) != 2 || tags[1][1] != "s1" {
		t.Fatalf("%#v", tags)
	}
}
