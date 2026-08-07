package mock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
)

func TestNew_SetValue_SubscribeManual(t *testing.T) {
	d := New()
	if d.Connected() {
		t.Fatal("expected disconnected")
	}
	n := 12.5
	d.SetValue("t1", core.Sample{ValueNum: &n, Quality: core.QualityGood})
	if err := d.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !d.Connected() {
		t.Fatal("expected connected")
	}
	out := make(chan core.Sample, 4)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go func() {
		_ = d.Subscribe(ctx, []core.Tag{
			{ID: "t1", DataType: core.ValueFloat64, Enabled: true},
			{ID: "missing", DataType: core.ValueFloat64, Enabled: true},
			{ID: "off", Enabled: false},
		}, out)
	}()
	got := map[string]core.Sample{}
	deadline := time.After(150 * time.Millisecond)
loop:
	for {
		select {
		case s := <-out:
			got[s.TagID] = s
			if len(got) >= 2 {
				break loop
			}
		case <-deadline:
			break loop
		}
	}
	if s, ok := got["t1"]; !ok || s.ValueNum == nil || *s.ValueNum != 12.5 {
		t.Fatalf("t1 %#v", got["t1"])
	}
	if s, ok := got["missing"]; !ok || s.Quality != core.QualityBad {
		t.Fatalf("missing %#v", got["missing"])
	}
	if _, ok := got["off"]; ok {
		t.Fatal("disabled tag should not emit")
	}
	if err := d.Disconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d.Connected() {
		t.Fatal("expected disconnected after Disconnect")
	}
}

func TestConnect_FailConnectAndConnectFn(t *testing.T) {
	d := New()
	d.FailConnectFor(time.Hour)
	if err := d.Connect(context.Background()); err == nil {
		t.Fatal("expected fail connect")
	}
	if d.Connected() {
		t.Fatal("should stay down")
	}

	d2 := New()
	d2.ConnectFn = func(ctx context.Context) error {
		return errors.New("boom")
	}
	if err := d2.Connect(context.Background()); err == nil {
		t.Fatal("expected ConnectFn error")
	}
	if d2.Connected() {
		t.Fatal("ConnectFn failure must leave disconnected")
	}
}

func TestSubscribe_NotConnected(t *testing.T) {
	d := New()
	err := d.Subscribe(context.Background(), nil, make(chan core.Sample))
	if err == nil {
		t.Fatal("expected not connected")
	}
}

func TestNewDemo_SynthesizeTypes(t *testing.T) {
	d := NewDemo(15 * time.Millisecond)
	if err := d.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	out := make(chan core.Sample, 32)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tags := []core.Tag{
		{ID: "f", DataType: core.ValueFloat64, Enabled: true},
		{ID: "i", DataType: core.ValueInt64, Enabled: true},
		{ID: "u", DataType: core.ValueUint, Enabled: true},
		{ID: "b", DataType: core.ValueBool, Enabled: true},
		{ID: "s", DataType: core.ValueString, Enabled: true},
		{ID: "dt", DataType: core.ValueDateTime, Enabled: true},
		{ID: "bad", DataType: core.ValueType("nope"), Enabled: true},
	}
	go func() { _ = d.Subscribe(ctx, tags, out) }()

	seen := map[string]core.Sample{}
	deadline := time.After(800 * time.Millisecond)
	for len(seen) < len(tags) {
		select {
		case s := <-out:
			seen[s.TagID] = s
		case <-deadline:
			t.Fatalf("timeout, seen=%v", keys(seen))
		}
	}
	if seen["f"].ValueNum == nil || seen["i"].ValueNum == nil || seen["u"].ValueNum == nil {
		t.Fatalf("numeric %#v", seen)
	}
	if seen["b"].ValueBool == nil || seen["s"].ValueText == nil || seen["dt"].ValueText == nil {
		t.Fatalf("scalar %#v", seen)
	}
	if seen["bad"].Quality != core.QualityBad {
		t.Fatalf("bad type %#v", seen["bad"])
	}
}

func keys(m map[string]core.Sample) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
