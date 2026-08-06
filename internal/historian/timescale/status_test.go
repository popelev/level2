package timescale

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiskFreeLocalPath(t *testing.T) {
	dir := t.TempDir()
	n, err := diskFree(dir)
	if err != nil {
		t.Fatalf("diskFree: %v", err)
	}
	if n <= 0 {
		t.Fatalf("expected free bytes > 0, got %d", n)
	}
	// Also works on a file path's volume (Windows drive / Linux mount).
	f := filepath.Join(dir, "marker")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	n2, err := diskFree(f)
	if err != nil {
		t.Fatalf("diskFree(file): %v", err)
	}
	if n2 <= 0 {
		t.Fatalf("expected free bytes > 0 for file path, got %d", n2)
	}
}

func TestResolveFreeBytesStatfs(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "")
	dir := t.TempDir()
	t.Setenv("LEVEL2_DB_DATA_PATH", dir)
	free, source, cap, diskTotal, diskAvail, diskPath, limit := resolveFreeBytes(1000, 50)
	if free == nil || *free < 0 {
		t.Fatalf("free=%v source=%s", free, source)
	}
	if diskTotal == nil || *diskTotal <= 0 {
		t.Fatalf("diskTotal=%v", diskTotal)
	}
	if diskAvail == nil || *diskAvail <= 0 {
		t.Fatalf("diskAvail=%v", diskAvail)
	}
	if diskPath != dir {
		t.Fatalf("diskPath=%q want %q", diskPath, dir)
	}
	if limit == nil || *limit != *diskTotal/2 {
		t.Fatalf("limit=%v diskTotal=%v", limit, diskTotal)
	}
	if cap == nil || *cap != *limit {
		t.Fatalf("cap=%v limit=%v", cap, limit)
	}
	if source != "under_limit" && source != "disk_avail" {
		t.Fatalf("source=%q", source)
	}
	// Same Statfs call feeds total + avail; free never exceeds avail.
	if *free > *diskAvail {
		t.Fatalf("free %d > diskAvail %d", *free, *diskAvail)
	}
}

func TestResolveFreeBytesEnvOverride(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "10000")
	t.Setenv("LEVEL2_DB_DATA_PATH", t.TempDir())
	free, source, cap, diskTotal, diskAvail, diskPath, limit := resolveFreeBytes(3000, 90)
	if source != "env_limit" || free == nil || *free != 7000 || cap == nil || *cap != 10000 {
		t.Fatalf("free=%v source=%s cap=%v", free, source, cap)
	}
	if limit == nil || *limit != 10000 || diskTotal == nil || *diskTotal != 10000 {
		t.Fatalf("limit=%v diskTotal=%v", limit, diskTotal)
	}
	if diskAvail != nil || diskPath != "" {
		t.Fatalf("env override should not set diskAvail/path, got avail=%v path=%q", diskAvail, diskPath)
	}
}

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
