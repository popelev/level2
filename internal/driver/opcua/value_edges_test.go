package opcua

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

func TestAsFloat64_AllNumeric(t *testing.T) {
	cases := []struct {
		in   any
		want float64
	}{
		{float32(1.5), 1.5},
		{float64(2.5), 2.5},
		{int16(-3), -3},
		{int32(4), 4},
		{int64(5), 5},
		{uint16(6), 6},
		{uint32(7), 7},
		{uint64(8), 8},
	}
	for _, tc := range cases {
		got, err := asFloat64(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("%T: got %v %v want %v", tc.in, got, err, tc.want)
		}
	}
}

func TestAsInt64_AllNumeric(t *testing.T) {
	cases := []struct {
		in   any
		want int64
	}{
		{int8(-1), -1},
		{int16(-2), -2},
		{int32(-3), -3},
		{int64(-4), -4},
		{uint8(5), 5},
		{uint16(6), 6},
		{uint32(7), 7},
		{uint64(8), 8},
	}
	for _, tc := range cases {
		got, err := asInt64(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("%T: got %v %v want %v", tc.in, got, err, tc.want)
		}
	}
	if _, err := asInt64(uint64(math.MaxUint64)); err == nil {
		t.Fatal("uint64 overflow")
	}
}

func TestAsUint64_AllNumeric(t *testing.T) {
	cases := []struct {
		in   any
		want uint64
	}{
		{uint8(1), 1},
		{uint16(2), 2},
		{uint32(3), 3},
		{uint64(4), 4},
		{int8(5), 5},
		{int16(6), 6},
		{int32(7), 7},
		{int64(8), 8},
	}
	for _, tc := range cases {
		got, err := asUint64(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("%T: got %v %v want %v", tc.in, got, err, tc.want)
		}
	}
	for _, neg := range []any{int8(-1), int16(-2), int64(-3)} {
		if _, err := asUint64(neg); err == nil {
			t.Fatalf("neg %T should fail", neg)
		}
	}
}

func TestAsByteSlice_Edges(t *testing.T) {
	b, ok := asByteSlice([]byte{1, 2, 3})
	if !ok || len(b) != 3 || b[0] != 1 {
		t.Fatalf("%v %v", b, ok)
	}
	arr := [2]byte{'a', 'b'}
	b, ok = asByteSlice(arr)
	if !ok || string(b) != "ab" {
		t.Fatalf("array %v %v", b, ok)
	}
	p := []byte{9}
	b, ok = asByteSlice(&p)
	if !ok || b[0] != 9 {
		t.Fatalf("ptr slice %v %v", b, ok)
	}
	var nilPtr *[]byte
	if _, ok := asByteSlice(nilPtr); ok {
		t.Fatal("nil ptr")
	}
	if _, ok := asByteSlice([]int{1, 2}); ok {
		t.Fatal("int slice")
	}
	if _, ok := asByteSlice(42); ok {
		t.Fatal("int")
	}
}

func TestAsDateTime_NumericAndErrors(t *testing.T) {
	when := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	ft := when.UnixNano()/100 + opcFiletimeEpoch

	if got, err := asDateTime(int32(1_700_000_000)); err != nil || got.Year() < 2020 {
		t.Fatalf("i32 %v %v", got, err)
	}
	if got, err := asDateTime(uint32(1_700_000_000)); err != nil || got.Year() < 2020 {
		t.Fatalf("u32 %v %v", got, err)
	}
	if got, err := asDateTime(float64(ft)); err != nil || !got.Equal(when) {
		t.Fatalf("f64 filetime %v %v", got, err)
	}
	var nilT *time.Time
	if _, err := asDateTime(nilT); err == nil {
		t.Fatal("nil *time.Time")
	}
	if _, err := asDateTime(struct{}{}); err == nil {
		t.Fatal("struct")
	}
	// []uint8 via reflection path
	raw := []uint8(when.Format(time.RFC3339))
	got, err := asDateTime(raw)
	if err != nil || got.Year() != 2026 {
		t.Fatalf("[]uint8 string %v %v", got, err)
	}
}

func TestBytesToDateTime_FiletimeAndFail(t *testing.T) {
	when := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	ft := when.UnixNano()/100 + opcFiletimeEpoch
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(ft))
	got, err := bytesToDateTime(buf)
	if err != nil || !got.Equal(when) {
		t.Fatalf("filetime bytes %v %v", got, err)
	}

	// Plausible LDT big-endian nanoseconds (after siemens DT BCD reject).
	ldtWant := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	be := make([]byte, 8)
	binary.BigEndian.PutUint64(be, uint64(ldtWant.UnixNano()))
	// Non-BCD high nibbles → skip siemens DT, then filetime may fail year check, then LDT.
	got, err = bytesToDateTime(be)
	if err != nil {
		t.Fatal(err)
	}
	if got.Year() < 1990 {
		t.Fatalf("LDT path year %v", got)
	}

	if _, err := bytesToDateTime([]byte{0xff, 0xfe}); err == nil {
		t.Fatal("short garbage")
	}
	got, err = bytesToDateTime([]byte("2026-08-06T12:00:00Z"))
	if err != nil || got.Year() != 2026 {
		t.Fatalf("string bytes %v %v", got, err)
	}
}

func TestSiemensDateAndTime_Invalid(t *testing.T) {
	if _, err := siemensDateAndTime([]byte{1, 2, 3}); err == nil {
		t.Fatal("short")
	}
	// Non-BCD nibble (>9)
	if _, err := siemensDateAndTime([]byte{0xAA, 1, 1, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("non-BCD")
	}
	// Invalid month 13 (BCD 0x13 → 13)
	if _, err := siemensDateAndTime([]byte{0x25, 0x13, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00}); err == nil {
		t.Fatal("bad month")
	}
}

func TestMapDataValue_Edges(t *testing.T) {
	now := time.Now().UTC()

	// Empty value
	_, err := mapDataValue(TagView{Tag: core.Tag{ID: "x", DataType: core.ValueFloat64}},
		&ua.DataValue{Status: ua.StatusOK}, now)
	if err == nil {
		t.Fatal("empty value")
	}

	// ServerTimestamp when Source empty
	bv, err := ua.NewVariant(true)
	if err != nil {
		t.Fatal(err)
	}
	srv := time.Unix(100, 0).UTC()
	s, err := mapDataValue(TagView{Tag: core.Tag{ID: "b", DataType: core.ValueBool}},
		&ua.DataValue{Status: ua.StatusOK, Value: bv, ServerTimestamp: srv}, now)
	if err != nil || !s.Time.Equal(srv) {
		t.Fatalf("server ts %#v %v", s, err)
	}

	// Wrong bool type
	fv, err := ua.NewVariant(float64(1))
	if err != nil {
		t.Fatal(err)
	}
	_, err = mapDataValue(TagView{Tag: core.Tag{ID: "b", DataType: core.ValueBool}},
		&ua.DataValue{Status: ua.StatusOK, Value: fv}, now)
	if err == nil {
		t.Fatal("bool from float")
	}

	// Uint path
	uv, err := ua.NewVariant(uint16(99))
	if err != nil {
		t.Fatal(err)
	}
	s, err = mapDataValue(TagView{Tag: core.Tag{ID: "u", DataType: core.ValueUint}},
		&ua.DataValue{Status: ua.StatusOK, Value: uv}, now)
	if err != nil || s.ValueNum == nil || *s.ValueNum != 99 {
		t.Fatalf("uint %#v %v", s, err)
	}

	// Float wrong type (bool → structure-ish error)
	_, err = mapDataValue(TagView{Tag: core.Tag{ID: "f", DataType: core.ValueFloat64}},
		&ua.DataValue{Status: ua.StatusOK, Value: bv}, now)
	if err == nil {
		t.Fatal("float from bool")
	}
}

func TestAsString_ViaByteSlice(t *testing.T) {
	arr := [3]byte{'x', 'y', 'z'}
	s, err := asString(arr)
	if err != nil || s != "xyz" {
		t.Fatalf("%q %v", s, err)
	}
}
