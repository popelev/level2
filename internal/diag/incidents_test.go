package diag

import (
	"testing"
	"time"
)

func TestIncidentTracker_CountWindow(t *testing.T) {
	tr := NewIncidentTracker(100, time.Hour)
	tr.Record(IncidentOPCDisconnect, "a")
	tr.Record(IncidentOPCDisconnect, "b")
	tr.Record(IncidentCollectorDown, "")
	tr.Record(IncidentDBWriteError, "")

	if got := tr.Count(IncidentOPCDisconnect, time.Hour); got != 2 {
		t.Fatalf("opc count=%d", got)
	}
	if got := tr.Count(IncidentCollectorDown, time.Hour); got != 1 {
		t.Fatalf("collector count=%d", got)
	}
	by := tr.CountByDevice(IncidentOPCDisconnect, time.Hour)
	if by["a"] != 1 || by["b"] != 1 {
		t.Fatalf("by device %#v", by)
	}
}

func TestIncidentTracker_PruneOld(t *testing.T) {
	tr := NewIncidentTracker(100, time.Hour)
	tr.mu.Lock()
	tr.events = []incident{
		{At: time.Now().UTC().Add(-2 * time.Hour), Kind: IncidentOPCDisconnect, DeviceID: "old"},
		{At: time.Now().UTC().Add(-30 * time.Minute), Kind: IncidentOPCDisconnect, DeviceID: "new"},
	}
	tr.mu.Unlock()

	if got := tr.Count(IncidentOPCDisconnect, time.Hour); got != 1 {
		t.Fatalf("want 1 recent, got %d", got)
	}
}

func TestIncidentTracker_RingCap(t *testing.T) {
	tr := NewIncidentTracker(3, time.Hour)
	for i := 0; i < 5; i++ {
		tr.Record(IncidentDBWriteError, "")
	}
	if got := tr.Count(IncidentDBWriteError, time.Hour); got != 3 {
		t.Fatalf("want capped 3, got %d", got)
	}
}
