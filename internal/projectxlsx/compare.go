package projectxlsx

import (
	"fmt"
	"strconv"

	"github.com/popelev/level2/internal/core"
)

// DiffRow is one comparison line (wide).
type DiffRow struct {
	Status   string `json:"status"` // same|added_in_b|removed_in_b|changed
	Kind     string `json:"kind"`   // server|tag
	DeviceID string `json:"device_id,omitempty"`
	ID       string `json:"id"`
	Field    string `json:"field,omitempty"`
	A        string `json:"a,omitempty"`
	B        string `json:"b,omitempty"`
}

// Compare returns rows for servers and tags between A and B.
func Compare(a, b []core.Device) []DiffRow {
	var out []DiffRow
	am := indexDevices(a)
	bm := indexDevices(b)
	ids := unionKeys(am, bm)
	for _, id := range ids {
		da, oka := am[id]
		db, okb := bm[id]
		if oka && !okb {
			out = append(out, DiffRow{Status: "removed_in_b", Kind: "server", ID: id, A: da.Endpoint})
			continue
		}
		if !oka && okb {
			out = append(out, DiffRow{Status: "added_in_b", Kind: "server", ID: id, B: db.Endpoint})
			continue
		}
		for _, f := range []struct{ name, av, bv string }{
			{"endpoint", da.Endpoint, db.Endpoint},
			{"username", da.Username, db.Username},
			{"security", da.Security, db.Security},
		} {
			if f.av != f.bv {
				out = append(out, DiffRow{Status: "changed", Kind: "server", ID: id, Field: f.name, A: f.av, B: f.bv})
			}
		}
	}

	at := indexTags(a)
	bt := indexTags(b)
	tkeys := unionKeys(at, bt)
	for _, k := range tkeys {
		ta, oka := at[k]
		tb, okb := bt[k]
		dev, tid := splitKey(k)
		if oka && !okb {
			out = append(out, DiffRow{Status: "removed_in_b", Kind: "tag", DeviceID: dev, ID: tid, A: ta.NodeID})
			continue
		}
		if !oka && okb {
			out = append(out, DiffRow{Status: "added_in_b", Kind: "tag", DeviceID: dev, ID: tid, B: tb.NodeID})
			continue
		}
		for _, f := range []struct{ name, av, bv string }{
			{"node_id", ta.NodeID, tb.NodeID},
			{"path", ta.Path, tb.Path},
			{"datatype", string(ta.DataType), string(tb.DataType)},
			{"enabled", strconv.FormatBool(ta.Enabled), strconv.FormatBool(tb.Enabled)},
			{"interval_ms", strconv.Itoa(ta.IntervalMs), strconv.Itoa(tb.IntervalMs)},
			{"writable", strconv.FormatBool(ta.Writable), strconv.FormatBool(tb.Writable)},
			{"simulate", strconv.FormatBool(ta.Simulate), strconv.FormatBool(tb.Simulate)},
		} {
			if f.av != f.bv {
				out = append(out, DiffRow{Status: "changed", Kind: "tag", DeviceID: dev, ID: tid, Field: f.name, A: f.av, B: f.bv})
			}
		}
	}
	return out
}

func DiffToSheets(rows []DiffRow) (servers, tags [][]string) {
	servers = [][]string{{"status", "id", "field", "a", "b"}}
	tags = [][]string{{"status", "device_id", "id", "field", "a", "b"}}
	for _, r := range rows {
		if r.Kind == "server" {
			servers = append(servers, []string{r.Status, r.ID, r.Field, r.A, r.B})
		} else {
			tags = append(tags, []string{r.Status, r.DeviceID, r.ID, r.Field, r.A, r.B})
		}
	}
	return servers, tags
}

func indexDevices(devs []core.Device) map[string]core.Device {
	m := map[string]core.Device{}
	for _, d := range devs {
		m[d.ID] = d
	}
	return m
}

func indexTags(devs []core.Device) map[string]core.Tag {
	m := map[string]core.Tag{}
	for _, d := range devs {
		for _, t := range d.Tags {
			m[d.ID+"\x00"+t.ID] = t
		}
	}
	return m
}

func unionKeys[V any](a, b map[string]V) []string {
	seen := map[string]bool{}
	var out []string
	for k := range a {
		seen[k] = true
		out = append(out, k)
	}
	for k := range b {
		if !seen[k] {
			out = append(out, k)
		}
	}
	// stable-ish
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func splitKey(k string) (deviceID, tagID string) {
	for i := 0; i < len(k); i++ {
		if k[i] == 0 {
			return k[:i], k[i+1:]
		}
	}
	return "", k
}

// Summary counts for preview.
func Summary(devs []core.Device) (servers, tags int) {
	servers = len(devs)
	for _, d := range devs {
		tags += len(d.Tags)
	}
	return
}

func FormatErr(errs []string, max int) string {
	if len(errs) == 0 {
		return ""
	}
	if len(errs) > max {
		return fmt.Sprintf("%s … (+%d)", errs[0], len(errs)-1)
	}
	return errs[0]
}
