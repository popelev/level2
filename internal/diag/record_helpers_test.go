package diag

import "testing"

func TestRecordHelpers_DefaultBuffer(t *testing.T) {
	b := NewBuffer(50)
	SetDefault(b)
	t.Cleanup(func() { SetDefault(nil) })

	OPCRead(LevelWarn, "dev", "tag", "poll failed", "detail")
	DBWrite(LevelInfo, "wrote", "batch", 3)
	OPCWrite(LevelInfo, "dev", "tag", "opc write ok", "node=ns=4;i=1")
	got := b.Query("all", false, 10)
	if len(got) != 3 {
		t.Fatalf("%#v", got)
	}
	if got[0].Category != CategoryOPCWrite || got[0].TagID != "tag" {
		t.Fatalf("newest %#v", got[0])
	}
	if got[1].Category != CategoryDBWrite || got[1].Count != 3 {
		t.Fatalf("mid %#v", got[1])
	}
	if got[2].Category != CategoryOPCRead || got[2].TagID != "tag" {
		t.Fatalf("older %#v", got[2])
	}
}

func TestBuffer_RingAndDefaults(t *testing.T) {
	b := NewBuffer(0) // → 2000 default max, but we only add a few
	if b.max != 2000 {
		t.Fatalf("max=%d", b.max)
	}
	small := NewBuffer(2)
	small.Add(Entry{Level: LevelInfo, Category: CategoryOPCRead, Message: "1"})
	small.Add(Entry{Level: LevelInfo, Category: CategoryOPCRead, Message: "2"})
	small.Add(Entry{Level: LevelInfo, Category: CategoryOPCRead, Message: "3"})
	all := small.Query("all", false, 10)
	if len(all) != 2 || all[0].Message != "3" || all[1].Message != "2" {
		t.Fatalf("%#v", all)
	}
}

func TestIncidentRecordHelpers(t *testing.T) {
	inc := NewIncidentTracker(0, 0)
	SetDefaultIncidents(inc)
	t.Cleanup(func() { SetDefaultIncidents(NewIncidentTracker(100, 0)) })

	RecordOPCDisconnect("d1")
	RecordCollectorDown()
	RecordDBWriteError()
	if inc.Count(IncidentOPCDisconnect, 0) != 1 {
		t.Fatal("opc")
	}
	if inc.Count(IncidentCollectorDown, 0) != 1 {
		t.Fatal("collector")
	}
	if inc.Count(IncidentDBWriteError, 0) != 1 {
		t.Fatal("db")
	}
	by := inc.CountByDevice(IncidentOPCDisconnect, 0)
	if by["d1"] != 1 {
		t.Fatalf("%v", by)
	}
	inc.Clear()
	if inc.Count(IncidentOPCDisconnect, 0) != 0 {
		t.Fatal("clear")
	}
	SetDefaultIncidents(nil) // no-op
}
