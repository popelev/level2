package timescale

import "testing"

func TestMaskDatabaseURL(t *testing.T) {
	in := "postgres://level2:s3cret@timescaledb:5432/level2?sslmode=disable"
	got := MaskDatabaseURL(in)
	if got != "postgres://level2:***@timescaledb:5432/level2?sslmode=disable" {
		t.Fatalf("masked=%q", got)
	}
	if MaskDatabaseURL("") != "" {
		t.Fatal("empty")
	}
	host, port, db, user, ssl := ParseDatabaseURL(in)
	if host != "timescaledb" || port != "5432" || db != "level2" || user != "level2" || ssl != "disable" {
		t.Fatalf("parse: %s %s %s %s %s", host, port, db, user, ssl)
	}
}
