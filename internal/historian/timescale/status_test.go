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
	if n < 0 {
		t.Fatalf("expected free bytes >= 0, got %d", n)
	}
	if n == 0 {
		// Some Docker/overlay mounts report Bavail=0; still exercise the syscall path.
		t.Logf("diskFree(%s)=0 (ok for constrained mounts)", dir)
	}
	// Re-query via the directory (Windows GetDiskFreeSpaceEx rejects bare file paths).
	f := filepath.Join(dir, "marker")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	n2, err := diskFree(filepath.Dir(f))
	if err != nil {
		t.Fatalf("diskFree(dir): %v", err)
	}
	if n2 < 0 {
		t.Fatalf("expected free bytes >= 0 for dir path, got %d", n2)
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
		t.Skipf("Statfs total unavailable for %s (skipping)", dir)
	}
	if diskAvail == nil || *diskAvail < 0 {
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

	// Over limit → free clamped to 0.
	free, source, _, _, _, _, _ = resolveFreeBytes(15000, 90)
	if source != "env_limit" || free == nil || *free != 0 {
		t.Fatalf("over limit: free=%v source=%s", free, source)
	}
}

func TestResolveCapacityLimitClampsPercent(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "")
	dir := t.TempDir()
	t.Setenv("LEVEL2_DB_DATA_PATH", dir)
	limitLow, total := resolveCapacityLimit(0) // treated as 90
	if total <= 0 || limitLow != total*90/100 {
		t.Fatalf("percent 0 → 90: limit=%d total=%d", limitLow, total)
	}
	limitHi, totalHi := resolveCapacityLimit(200) // clamped to 100
	if totalHi != total || limitHi != total {
		t.Fatalf("percent 200 → 100: limit=%d total=%d", limitHi, totalHi)
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
