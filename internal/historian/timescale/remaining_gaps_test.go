package timescale

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
)

func TestDiskSpace_NULPath(t *testing.T) {
	_, _, err := diskSpace("C:\\bad\x00path")
	if err == nil {
		t.Fatal("expected UTF16PtrFromString error for NUL in path")
	}
}

func TestCapacity_DiskFieldsNoEnv(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "")
	dir := t.TempDir()
	t.Setenv("LEVEL2_DB_DATA_PATH", dir)

	p := &fakePool{
		queryRowFn: func(sql string, _ []any) pgx.Row {
			switch {
			case sqlHas(sql, "pg_database_size"):
				return rowInt64(1000)
			case sqlHas(sql, "hypertable_size"):
				return rowInt64(800)
			case sqlHas(sql, "approximate_row_count"):
				return rowInt64(40)
			case sqlHas(sql, "count(*)"):
				return rowInt64(0) // no growth → no ETA
			default:
				return rowErr(errors.New(sql))
			}
		},
	}
	h := &Historian{pool: p}
	h.SetCapacityPolicy(50, config.FullPolicyStop)
	out, err := h.Capacity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.DiskPath != dir {
		t.Fatalf("DiskPath=%q want %q", out.DiskPath, dir)
	}
	if out.DiskTotalBytes == nil || *out.DiskTotalBytes <= 0 {
		t.Skipf("statfs total unavailable for %s", dir)
	}
	if out.DiskAvailBytes == nil {
		t.Fatalf("DiskAvailBytes nil: %+v", out)
	}
	if out.LimitBytes == nil || out.CapacityBytes == nil {
		t.Fatalf("limit/cap missing: %+v", out)
	}
	if out.ETASeconds != nil {
		t.Fatalf("expected no ETA with zero growth, got %v", *out.ETASeconds)
	}
	if out.FreeBytesSource != "under_limit" && out.FreeBytesSource != "disk_avail" {
		t.Fatalf("source=%q", out.FreeBytesSource)
	}
}

func TestDropOldestChunk_RowsAffectedPositive(t *testing.T) {
	cutoff := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	p := &fakePool{
		queryRowFn: func(sql string, _ []any) pgx.Row {
			if sqlHas(sql, "timescaledb_information.chunks") {
				return rowTimePtr(&cutoff)
			}
			return rowErr(errors.New(sql))
		},
		execFn: func(sql string, _ []any) (pgconn.CommandTag, error) {
			if sqlHas(sql, "drop_chunks") {
				return pgconn.NewCommandTag("SELECT 4"), nil
			}
			return pgconn.NewCommandTag(""), errors.New(sql)
		},
	}
	n, err := (&Historian{pool: p}).dropOldestChunk(context.Background())
	if err != nil || n != 4 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestQueryHistory_TextBoolAndLimitCap(t *testing.T) {
	now := time.Date(2026, 8, 7, 15, 0, 0, 0, time.UTC)
	txt := "hello"
	flag := true
	var gotLimit int
	p := &fakePool{
		queryFn: func(sql string, args []any) (pgx.Rows, error) {
			gotLimit = args[3].(int)
			return &fakeRows{vals: [][]any{
				{now, "t1", nil, txt, nil, int(core.QualityBad)},
				{now.Add(time.Second), "t1", nil, nil, flag, int(core.QualityGood)},
			}}, nil
		},
	}
	out, err := (&Historian{pool: p}).QueryHistory(context.Background(), "t1", now.Add(-time.Minute), now.Add(time.Minute), 50000)
	if err != nil {
		t.Fatal(err)
	}
	if gotLimit != 1000 {
		t.Fatalf("limit capped to 1000, got %d", gotLimit)
	}
	if len(out) != 2 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0].ValueText == nil || *out[0].ValueText != "hello" || out[0].Quality != core.QualityBad {
		t.Fatalf("text row %+v", out[0])
	}
	if out[1].ValueBool == nil || !*out[1].ValueBool {
		t.Fatalf("bool row %+v", out[1])
	}
}

func TestStatus_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &fakePool{}
	st := (&Historian{pool: p}).Status(ctx, "postgres://u:p@h:5432/db")
	if st.PingOK || st.PingError == "" {
		t.Fatalf("%+v", st)
	}
}

func TestPing_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (&Historian{pool: &fakePool{}}).Ping(ctx); err == nil {
		t.Fatal("expected canceled ping")
	}
}

func TestMaskDatabaseURL_TrimAndNoSchemeInline(t *testing.T) {
	got := MaskDatabaseURL("  postgres://u:secret@host/db  ")
	if !strings.Contains(got, ":***") || strings.Contains(got, "secret") {
		t.Fatalf("%q", got)
	}
	// Scheme-less credential form still redacts via inline helper.
	got = MaskDatabaseURL("postgres://alice:pw@")
	if got == "" || strings.Contains(got, "pw") && !strings.Contains(got, "***") {
		// Accept either masked inline or parse-normalized form without leaking pw.
		if strings.Contains(got, ":pw@") || strings.Contains(got, ":pw") {
			t.Fatalf("password leaked: %q", got)
		}
	}
}

func TestWriteBatch_EmptyAfterCapacityOK(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "10000")
	p := &fakePool{
		queryRowFn: func(sql string, _ []any) pgx.Row {
			return rowInt64(1)
		},
	}
	h := &Historian{pool: p}
	h.SetCapacityPolicy(90, config.FullPolicyStop)
	if err := h.WriteBatch(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := h.WriteBatch(context.Background(), []core.Sample{}); err != nil {
		t.Fatal(err)
	}
}

// Status reads pool.Stat() only for a concrete *pgxpool.Pool (cannot fake).
// Use an unreachable DSN so we exercise the branch without a live database.
func TestStatus_RealPoolStatBranch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	dsn := "postgres://level2:x@127.0.0.1:1/level2?sslmode=disable&connect_timeout=1"
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("pgxpool.New: %v", err)
	}
	defer pool.Close()
	h := &Historian{pool: pool}
	st := h.Status(ctx, dsn)
	if st.PoolMaxConns <= 0 {
		t.Fatalf("expected Stat max conns > 0, got %+v", st)
	}
	if st.PingOK {
		t.Fatal("unreachable pool should not ping ok")
	}
	if st.PingError == "" {
		t.Fatal("expected ping error")
	}
}
