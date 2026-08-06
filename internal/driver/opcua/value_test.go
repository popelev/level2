package opcua

import (
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

func TestMapFloat(t *testing.T) {
	tag := TagView{Tag: core.Tag{ID: "t", DataType: core.ValueFloat64}}
	v, err := ua.NewVariant(float32(90.5))
	if err != nil {
		t.Fatal(err)
	}
	dv := &ua.DataValue{Status: ua.StatusOK, Value: v}
	s, err := mapDataValue(tag, dv, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if s.ValueNum == nil || *s.ValueNum != 90.5 {
		t.Fatalf("got %#v", s.ValueNum)
	}
}

func TestMapString(t *testing.T) {
	tag := TagView{Tag: core.Tag{ID: "u", DataType: core.ValueString}}
	v, err := ua.NewVariant("%")
	if err != nil {
		t.Fatal(err)
	}
	dv := &ua.DataValue{Status: ua.StatusOK, Value: v}
	s, err := mapDataValue(tag, dv, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if s.ValueText == nil || *s.ValueText != "%" {
		t.Fatalf("got %#v", s.ValueText)
	}
}

func TestMapDateTime(t *testing.T) {
	tag := TagView{Tag: core.Tag{ID: "ts", DataType: core.ValueDateTime}}
	when := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	v, err := ua.NewVariant(when)
	if err != nil {
		t.Fatal(err)
	}
	dv := &ua.DataValue{Status: ua.StatusOK, Value: v}
	s, err := mapDataValue(tag, dv, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if s.Quality != core.QualityGood {
		t.Fatalf("quality %#v", s.Quality)
	}
	if s.ValueText == nil || *s.ValueText == "" {
		t.Fatalf("got %#v", s.ValueText)
	}
	got, err := time.Parse(time.RFC3339Nano, *s.ValueText)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(when) {
		t.Fatalf("parsed %v want %v", got, when)
	}
}

func TestMapDateTimeFromString(t *testing.T) {
	tag := TagView{Tag: core.Tag{ID: "ts", DataType: core.ValueDateTime}}
	raw := "2026-08-06T12:00:00Z"
	v, err := ua.NewVariant(raw)
	if err != nil {
		t.Fatal(err)
	}
	dv := &ua.DataValue{Status: ua.StatusOK, Value: v}
	s, err := mapDataValue(tag, dv, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if s.Quality != core.QualityGood || s.ValueText == nil {
		t.Fatalf("got %#v err=%v", s, err)
	}
	got, err := time.Parse(time.RFC3339Nano, *s.ValueText)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("parsed %v want %v", got, want)
	}
}

func TestAsDateTimeVariants(t *testing.T) {
	when := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	ft := when.UnixNano()/100 + opcFiletimeEpoch

	type namedFT int64 // mimics ua.DateTime (FILETIME-backed named int64)

	cases := []struct {
		name string
		in   any
	}{
		{"time.Time", when},
		{"*time.Time", &when},
		{"int64 FILETIME", ft},
		{"uint64 FILETIME", uint64(ft)},
		{"named int64 FILETIME", namedFT(ft)},
		{"RFC3339 string", when.Format(time.RFC3339)},
		{"RFC3339Nano string", when.Format(time.RFC3339Nano)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := asDateTime(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(when) {
				t.Fatalf("got %v want %v", got, when)
			}
		})
	}

	// mapDataValue must emit Good + ValueText for FILETIME-backed values.
	tag := TagView{Tag: core.Tag{ID: "ft", DataType: core.ValueDateTime}}
	// Build DataValue via time.Time variant, then also exercise asDateTime→sample path for int64.
	tm, err := asDateTime(ft)
	if err != nil {
		t.Fatal(err)
	}
	v, err := ua.NewVariant(tm)
	if err != nil {
		t.Fatal(err)
	}
	s, err := mapDataValue(tag, &ua.DataValue{Status: ua.StatusOK, Value: v}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if s.Quality != core.QualityGood || s.ValueText == nil || *s.ValueText == "" {
		t.Fatalf("sample %#v", s)
	}
}

func TestMapUint(t *testing.T) {
	tag := TagView{Tag: core.Tag{ID: "u", DataType: core.ValueUint}}
	v, err := ua.NewVariant(uint32(42))
	if err != nil {
		t.Fatal(err)
	}
	dv := &ua.DataValue{Status: ua.StatusOK, Value: v}
	s, err := mapDataValue(tag, dv, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if s.ValueNum == nil || *s.ValueNum != 42 {
		t.Fatalf("got %#v", s.ValueNum)
	}
}

func TestMapStructureStatus(t *testing.T) {
	tag := TagView{Tag: core.Tag{ID: "s", DataType: core.ValueFloat64}}
	dv := &ua.DataValue{Status: ua.StatusCode(0x80110000)} // BadDataTypeIDUnknown
	_, err := mapDataValue(tag, dv, time.Now().UTC())
	if err == nil {
		t.Fatal("expected structure error")
	}
}
