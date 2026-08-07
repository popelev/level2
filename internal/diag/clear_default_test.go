package diag

import "testing"

func TestBuffer_Clear(t *testing.T) {
	b := NewBuffer(10)
	b.Add(Entry{Level: LevelInfo, Category: CategoryOPCRead, Message: "a"})
	b.Add(Entry{Level: LevelWarn, Category: CategoryDBWrite, Message: "b"})
	b.Clear()
	if got := b.Query("all", false, 10); len(got) != 0 {
		t.Fatalf("after clear: %#v", got)
	}
}

func TestDefaultIncidents_Accessor(t *testing.T) {
	if DefaultIncidents() == nil {
		t.Fatal("process default should be non-nil")
	}
	prev := DefaultIncidents()
	tr := NewIncidentTracker(10, 0)
	SetDefaultIncidents(tr)
	t.Cleanup(func() { SetDefaultIncidents(prev) })
	if DefaultIncidents() != tr {
		t.Fatal("DefaultIncidents should return set tracker")
	}
	// nil is ignored (keeps previous tracker)
	SetDefaultIncidents(nil)
	if DefaultIncidents() != tr {
		t.Fatal("SetDefaultIncidents(nil) must be no-op")
	}
}

func TestRecord_NilDefaultNoPanic(t *testing.T) {
	SetDefault(nil)
	t.Cleanup(func() {})
	OPCRead(LevelInfo, "d", "t", "msg", "")
	DBWrite(LevelInfo, "msg", "", 1)
}
