package timescale

import (
	"context"
	"testing"
)

func TestStatus_NilHistorian(t *testing.T) {
	var h *Historian
	st := h.Status(context.Background(), "postgres://user:secret@db:5432/l2?sslmode=disable")
	if st.PingError != "historian not configured" {
		t.Fatalf("%#v", st)
	}
	if st.Host != "db" || st.Port != "5432" || st.Database != "l2" || st.User != "user" {
		t.Fatalf("%#v", st)
	}
	if st.URLMasked == "" || st.URLMasked == "postgres://user:secret@db:5432/l2?sslmode=disable" {
		t.Fatalf("masked=%q", st.URLMasked)
	}
	if st.Connected || st.PingOK {
		t.Fatal("should not be connected")
	}
}

func TestCapacity_NilHistorian(t *testing.T) {
	var h *Historian
	_, err := h.Capacity(context.Background())
	if err == nil || err.Error() != "historian not configured" {
		t.Fatalf("%v", err)
	}
	h2 := &Historian{}
	_, err = h2.Capacity(context.Background())
	if err == nil || err.Error() != "historian not configured" {
		t.Fatalf("nil pool: %v", err)
	}
}

func TestPing_NilSafe(t *testing.T) {
	var h *Historian
	if err := h.Ping(context.Background()); err == nil {
		t.Fatal("expected ping error on nil historian")
	}
	h2 := &Historian{}
	if err := h2.Ping(context.Background()); err == nil {
		t.Fatal("expected ping error on nil pool")
	}
}
