package opcua

import (
	"encoding/binary"
	"testing"
	"time"
)

type withTimeMethod struct{ t time.Time }

func (w withTimeMethod) Time() time.Time { return w.t }

func TestSiemensLDT(t *testing.T) {
	want := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	nano := want.UnixNano()
	be := make([]byte, 8)
	binary.BigEndian.PutUint64(be, uint64(nano))
	got, err := siemensLDT(be)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(want) {
		t.Fatalf("BE got %v want %v", got, want)
	}

	if _, err := siemensLDT([]byte{1, 2, 3}); err == nil {
		t.Fatal("short LDT")
	}
	if _, err := siemensLDT(make([]byte, 8)); err == nil {
		t.Fatal("all-zero LDT should be out of range")
	}
}

func TestParseDateTimeString_Edges(t *testing.T) {
	if _, err := parseDateTimeString(""); err == nil {
		t.Fatal("empty")
	}
	if _, err := parseDateTimeString("not-a-date"); err == nil {
		t.Fatal("garbage")
	}
	got, err := parseDateTimeString("2026-08-06 12:00:00")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v", got)
	}
	// unix seconds as string
	got, err = parseDateTimeString("1690000000")
	if err != nil {
		t.Fatal(err)
	}
	if got.Year() < 2020 {
		t.Fatalf("unix seconds: %v", got)
	}
}

func TestTimeFromReflect(t *testing.T) {
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got, ok := timeFromReflect(withTimeMethod{t: when})
	if !ok || !got.Equal(when) {
		t.Fatalf("Time() method: ok=%v got=%v", ok, got)
	}
	if _, ok := timeFromReflect(nil); ok {
		t.Fatal("nil")
	}
	var p *int64
	if _, ok := timeFromReflect(p); ok {
		t.Fatal("nil pointer")
	}
	n := when.Unix()
	got, ok = timeFromReflect(&n)
	if !ok || got.IsZero() {
		t.Fatalf("int64 ptr: ok=%v got=%v", ok, got)
	}
	if _, ok := timeFromReflect("nope"); ok {
		t.Fatal("string should fail reflect path")
	}
}

func TestNumericToTimeAndFiletime(t *testing.T) {
	if !numericToTime(0).IsZero() {
		t.Fatal("zero")
	}
	if !filetimeToTime(0).IsZero() {
		t.Fatal("filetime zero")
	}
	milli := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC).UnixMilli()
	got := numericToTime(milli)
	if got.Year() != 2026 {
		t.Fatalf("millis %v", got)
	}
}
