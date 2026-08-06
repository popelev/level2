package timescale

import "testing"

func TestMaskDatabaseURL_Variants(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"postgres://user@host:5432/db", "postgres://user@host:5432/db"},
		{"postgres://u:p@h/db", "postgres://u:***@h/db"},
		{"postgresql://admin:hunter2@db.local:6432/level2?sslmode=require",
			"postgresql://admin:***@db.local:6432/level2?sslmode=require"},
		{"not-a-url", "not-a-url"},
		{"user:pass@host", "user:pass@host"}, // no scheme → unchanged by maskInline
	}
	for _, tc := range cases {
		got := MaskDatabaseURL(tc.in)
		if got != tc.want {
			t.Fatalf("in=%q got=%q want=%q", tc.in, got, tc.want)
		}
	}
}

func TestMaskInlinePassword(t *testing.T) {
	got := maskInlinePassword("postgres://alice:secret@host/db")
	if got != "postgres://alice:***@host/db" {
		t.Fatalf("%q", got)
	}
	if maskInlinePassword("nope") != "nope" {
		t.Fatal("no @")
	}
	if maskInlinePassword("postgres://alice@host/db") != "postgres://alice@host/db" {
		t.Fatal("no password")
	}
}

func TestParseDatabaseURL_Variants(t *testing.T) {
	host, port, db, user, ssl := ParseDatabaseURL("postgres://me:x@dbhost/mydb")
	if host != "dbhost" || port != "5432" || db != "mydb" || user != "me" || ssl != "" {
		t.Fatalf("%s %s %s %s %s", host, port, db, user, ssl)
	}
	host, port, db, user, ssl = ParseDatabaseURL("postgres://u:p@[::1]:9999/a/b?sslmode=verify-full")
	if host != "::1" || port != "9999" || db != "a" || user != "u" || ssl != "verify-full" {
		t.Fatalf("%s %s %s %s %s", host, port, db, user, ssl)
	}
	host, port, db, user, ssl = ParseDatabaseURL("")
	if host != "" || port != "" || db != "" || user != "" || ssl != "" {
		t.Fatalf("empty parse leaked values")
	}
	host, port, db, user, ssl = ParseDatabaseURL("://bad")
	if host != "" {
		t.Fatalf("bad url host=%q", host)
	}
}

func TestDBDiskPathsEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEVEL2_DB_DATA_PATH", dir)
	paths := dbDiskPaths()
	if len(paths) != 1 || paths[0] != dir {
		t.Fatalf("%v", paths)
	}
	t.Setenv("LEVEL2_DB_DATA_PATH", "")
	paths = dbDiskPaths()
	if len(paths) < 1 {
		t.Fatal("defaults empty")
	}
}

func TestResolveFreeBytes_OverLimitUnderDisk(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "")
	dir := t.TempDir()
	t.Setenv("LEVEL2_DB_DATA_PATH", dir)
	// used huge → free clamped; still returns source.
	free, source, _, _, diskAvail, _, limit := resolveFreeBytes(1<<62, 50)
	if free == nil || *free != 0 {
		t.Fatalf("free=%v source=%s", free, source)
	}
	if limit == nil || *limit <= 0 {
		t.Skipf("no disk total for %s", dir)
	}
	if diskAvail == nil {
		t.Fatalf("diskAvail nil source=%s", source)
	}
	if source != "under_limit" && source != "disk_avail" {
		t.Fatalf("source=%q", source)
	}
}
