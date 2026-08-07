package diag

import (
	"testing"
	"time"
)

func TestIncidentTracker_NilAndEmptyKind(t *testing.T) {
	var tr *IncidentTracker
	tr.Clear() // no panic
	tr.Record(IncidentOPCDisconnect, "x")
	if tr.Count(IncidentOPCDisconnect, time.Hour) != 0 {
		t.Fatal("nil Count")
	}
	if len(tr.CountByDevice(IncidentOPCDisconnect, time.Hour)) != 0 {
		t.Fatal("nil CountByDevice")
	}

	live := NewIncidentTracker(10, time.Hour)
	live.Record("", "ignored")
	if live.Count(IncidentOPCDisconnect, 0) != 0 {
		t.Fatal("empty kind must be ignored")
	}
	live.Record(IncidentOPCDisconnect, "")
	by := live.CountByDevice(IncidentOPCDisconnect, 0)
	if by["_"] != 1 {
		t.Fatalf("empty device → underscore: %#v", by)
	}
}
