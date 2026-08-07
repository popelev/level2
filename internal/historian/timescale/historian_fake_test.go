package timescale

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
)

func TestNew_InvalidURL(t *testing.T) {
	_, err := New(context.Background(), "://bad")
	if err == nil {
		t.Fatal("expected error for invalid database URL")
	}
}

func TestNew_PingFails(t *testing.T) {
	// Valid-looking URL that cannot be reached → pool may create then ping fails,
	// or pool creation fails; either way New must error.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := New(ctx, "postgres://level2:x@127.0.0.1:1/level2?sslmode=disable&connect_timeout=1")
	if err == nil {
		t.Fatal("expected unreachable db error")
	}
}

func TestCloseAndPing_WithFake(t *testing.T) {
	p := &fakePool{}
	h := &Historian{pool: p}
	h.SetCapacityPolicy(90, config.FullPolicyStop)
	if err := h.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !p.closed {
		t.Fatal("pool not closed")
	}
	if err := h.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	p.pingErr = errors.New("down")
	if err := h.Ping(context.Background()); err == nil {
		t.Fatal("expected ping error")
	}
}

func TestEnsureSchema_OKAndError(t *testing.T) {
	var calls []string
	p := &fakePool{
		execFn: func(sql string, _ []any) (pgconn.CommandTag, error) {
			calls = append(calls, sql)
			if strings.Contains(sql, "create_hypertable") {
				return pgconn.NewCommandTag(""), errors.New("no timescale")
			}
			return pgconn.NewCommandTag("OK"), nil
		},
	}
	h := &Historian{pool: p}
	if err := h.EnsureSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(calls) < 2 {
		t.Fatalf("expected schema + hypertable attempts, got %d", len(calls))
	}

	p2 := &fakePool{
		execFn: func(sql string, _ []any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag(""), errors.New("ddl denied")
		},
	}
	if err := (&Historian{pool: p2}).EnsureSchema(context.Background()); err == nil {
		t.Fatal("expected EnsureSchema error")
	}
}

func TestWriteBatch_InsertsAndPropagatesExecError(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "1000000")
	var gotSQL string
	var gotArgs []any
	p := &fakePool{
		queryRowFn: func(sql string, _ []any) pgx.Row {
			if sqlHas(sql, "pg_database_size") {
				return rowInt64(100) // under limit
			}
			return rowErr(errors.New("unexpected"))
		},
		execFn: func(sql string, args []any) (pgconn.CommandTag, error) {
			gotSQL = sql
			gotArgs = args
			return pgconn.NewCommandTag("INSERT 0 2"), nil
		},
	}
	h := &Historian{pool: p}
	h.SetCapacityPolicy(90, config.FullPolicyStop)
	n := 1.25
	txt := "hi"
	b := true
	now := time.Now().UTC()
	err := h.WriteBatch(context.Background(), []core.Sample{
		{Time: now, TagID: "a", ValueNum: &n, Quality: core.QualityGood},
		{Time: now.Add(time.Second), TagID: "b", ValueText: &txt, ValueBool: &b, Quality: core.QualityBad},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotSQL, "INSERT INTO collector.samples") {
		t.Fatalf("sql=%q", gotSQL)
	}
	if !strings.Contains(gotSQL, "($1,$2,$3,$4,$5,$6),($7,$8,$9,$10,$11,$12)") {
		t.Fatalf("placeholders=%q", gotSQL)
	}
	if len(gotArgs) != 12 {
		t.Fatalf("args=%d", len(gotArgs))
	}

	p.execFn = func(string, []any) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag(""), errors.New("insert fail")
	}
	if err := h.WriteBatch(context.Background(), []core.Sample{{TagID: "x"}}); err == nil {
		t.Fatal("expected insert error")
	}
}

func TestQueryHistory_DefaultsScanAndErrors(t *testing.T) {
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	num := 3.5
	p := &fakePool{
		queryFn: func(sql string, args []any) (pgx.Rows, error) {
			if len(args) != 4 {
				t.Fatalf("args=%v", args)
			}
			limit := args[3].(int)
			if limit != 1000 {
				t.Fatalf("default limit=%d", limit)
			}
			return &fakeRows{vals: [][]any{
				{now, "tag1", num, nil, nil, int(core.QualityGood)},
			}}, nil
		},
	}
	h := &Historian{pool: p}
	out, err := h.QueryHistory(context.Background(), "tag1", time.Time{}, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].TagID != "tag1" || out[0].ValueNum == nil || *out[0].ValueNum != 3.5 {
		t.Fatalf("%+v", out)
	}

	p.queryFn = func(string, []any) (pgx.Rows, error) {
		return nil, errors.New("query boom")
	}
	if _, err := h.QueryHistory(context.Background(), "t", now.Add(-time.Hour), now, 50); err == nil {
		t.Fatal("expected query error")
	}

	p.queryFn = func(string, []any) (pgx.Rows, error) {
		return &fakeRows{
			vals: [][]any{{now, "t", nil, nil, nil, 0}},
			err:  errors.New("rows err"),
		}, nil
	}
	if _, err := h.QueryHistory(context.Background(), "t", now.Add(-time.Hour), now, 20000); err == nil {
		t.Fatal("expected rows.Err")
	}

	p.queryFn = func(string, []any) (pgx.Rows, error) {
		return &fakeRows{vals: [][]any{
			{now, "t", "bad-type", nil, nil, 0},
		}}, nil
	}
	if _, err := h.QueryHistory(context.Background(), "t", now.Add(-time.Hour), now, 10); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestStatus_WithFakePool(t *testing.T) {
	p := &fakePool{
		pingErr: errors.New("ping fail"),
	}
	h := &Historian{pool: p}
	st := h.Status(context.Background(), "postgres://u:secret@db:5432/l2?sslmode=disable")
	if st.PingOK || st.Connected || st.PingError == "" {
		t.Fatalf("%+v", st)
	}

	p.pingErr = nil
	p.queryRowFn = func(sql string, _ []any) pgx.Row {
		switch {
		case sqlHas(sql, "server_version"):
			return rowString("16.1")
		case sqlHas(sql, "timescaledb"):
			return rowString("2.14.0")
		default:
			return rowErr(errors.New("unexpected " + sql))
		}
	}
	st = h.Status(context.Background(), "postgres://u:secret@db:5432/l2?sslmode=disable")
	if !st.PingOK || !st.Connected || st.ServerVersion != "16.1" || st.TimescaleVer != "2.14.0" {
		t.Fatalf("%+v", st)
	}
}

func TestCapacity_WithFakePool(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "10000")
	p := &fakePool{
		queryRowFn: func(sql string, _ []any) pgx.Row {
			switch {
			case sqlHas(sql, "pg_database_size"):
				return rowInt64(4000)
			case sqlHas(sql, "hypertable_size"):
				return rowErr(errors.New("no hypertable"))
			case sqlHas(sql, "pg_total_relation_size"):
				return rowInt64(2000)
			case sqlHas(sql, "approximate_row_count"):
				return rowErr(errors.New("no approx"))
			case sqlHas(sql, "reltuples"):
				return rowInt64(100)
			case sqlHas(sql, "count(*)"):
				return rowInt64(60) // → 0.2 samples/sec over 300s window
			default:
				return rowErr(errors.New("unexpected " + sql))
			}
		},
	}
	h := &Historian{pool: p}
	h.SetCapacityPolicy(90, config.FullPolicyStop)
	out, err := h.Capacity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.DatabaseSizeBytes != 4000 || out.SamplesSizeBytes != 2000 || out.SamplesApproxRows != 100 {
		t.Fatalf("%+v", out)
	}
	if out.SamplesLast5Min != 60 || out.FreeBytesSource != "env_limit" {
		t.Fatalf("%+v", out)
	}
	if out.FreeBytes == nil || *out.FreeBytes != 6000 || out.ETASeconds == nil {
		t.Fatalf("free/eta %+v", out)
	}
	if out.AvgSampleBytes != 20 || out.GrowthBytesPerSec <= 0 {
		t.Fatalf("growth %+v", out)
	}

	// Negative approx rows clamped; zero rows → default avg bytes; over limit.
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "1000")
	p.queryRowFn = func(sql string, _ []any) pgx.Row {
		switch {
		case sqlHas(sql, "pg_database_size"):
			return rowInt64(5000)
		case sqlHas(sql, "hypertable_size"):
			return rowInt64(0)
		case sqlHas(sql, "approximate_row_count"):
			return rowInt64(-5)
		case sqlHas(sql, "count(*)"):
			return rowInt64(0)
		default:
			return rowErr(errors.New(sql))
		}
	}
	out, err = h.Capacity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.SamplesApproxRows != 0 || out.AvgSampleBytes != 120 || !out.UsedOverLimit {
		t.Fatalf("%+v", out)
	}
	if out.FreeBytes == nil || *out.FreeBytes != 0 {
		t.Fatalf("free=%v", out.FreeBytes)
	}

	p.queryRowFn = func(sql string, _ []any) pgx.Row {
		if sqlHas(sql, "pg_database_size") {
			return rowErr(errors.New("size fail"))
		}
		return rowErr(errors.New("x"))
	}
	if _, err := h.Capacity(context.Background()); err == nil {
		t.Fatal("expected size error")
	}
}

func TestWipeSamples_TruncateAndDeleteFallback(t *testing.T) {
	p := &fakePool{
		queryRowFn: func(sql string, _ []any) pgx.Row {
			if sqlHas(sql, "approximate_row_count") {
				return rowInt64(42)
			}
			return rowErr(errors.New("unexpected"))
		},
		execFn: func(sql string, _ []any) (pgconn.CommandTag, error) {
			if sqlHas(sql, "truncate") {
				return pgconn.NewCommandTag("TRUNCATE"), nil
			}
			return pgconn.NewCommandTag(""), errors.New("nope")
		},
	}
	out, err := (&Historian{pool: p}).WipeSamples(context.Background())
	if err != nil || out.Method != "truncate" || out.ApproxRows != 42 {
		t.Fatalf("%+v %v", out, err)
	}

	p.queryRowFn = func(sql string, _ []any) pgx.Row {
		if sqlHas(sql, "approximate_row_count") {
			return rowErr(errors.New("no approx"))
		}
		if sqlHas(sql, "reltuples") {
			return rowInt64(-3)
		}
		return rowErr(errors.New(sql))
	}
	p.execFn = func(sql string, _ []any) (pgconn.CommandTag, error) {
		if sqlHas(sql, "truncate") {
			return pgconn.NewCommandTag(""), errors.New("denied")
		}
		if sqlHas(sql, "delete") {
			return pgconn.NewCommandTag("DELETE 7"), nil
		}
		return pgconn.NewCommandTag(""), errors.New(sql)
	}
	out, err = (&Historian{pool: p}).WipeSamples(context.Background())
	if err != nil || out.Method != "delete" || out.ApproxRows != 7 {
		t.Fatalf("%+v %v", out, err)
	}

	p.execFn = func(sql string, _ []any) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag(""), errors.New("fail " + sql)
	}
	if _, err := (&Historian{pool: p}).WipeSamples(context.Background()); err == nil {
		t.Fatal("expected wipe error")
	}
}

func TestEnforceCapacity_Policies(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "1000")

	cases := []struct {
		name   string
		policy string
		used   int64
		wantIs error
	}{
		{name: "under_limit", policy: config.FullPolicyStop, used: 100},
		{name: "stop", policy: config.FullPolicyStop, used: 2000, wantIs: ErrCapacityHalt},
		{name: "rotate", policy: config.FullPolicyRotate, used: 2000, wantIs: ErrCapacityHalt},
		{name: "expand", policy: config.FullPolicyExpandLimit, used: 2000, wantIs: ErrCapacityHalt},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakePool{
				queryRowFn: func(sql string, _ []any) pgx.Row {
					if sqlHas(sql, "pg_database_size") {
						return rowInt64(tc.used)
					}
					return rowErr(errors.New(sql))
				},
			}
			h := &Historian{pool: p}
			h.SetCapacityPolicy(90, tc.policy)
			err := h.enforceCapacity(context.Background())
			if tc.wantIs == nil {
				if err != nil {
					t.Fatalf("unexpected err %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantIs) {
				t.Fatalf("err=%v", err)
			}
		})
	}

	// Cannot evaluate limit (no env, bad disk path) → allow write.
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "")
	t.Setenv("LEVEL2_DB_DATA_PATH", `\\?\Level2SurelyMissingDiskPath\nope`)
	p := &fakePool{
		queryRowFn: func(sql string, _ []any) pgx.Row {
			return rowInt64(999999)
		},
	}
	h := &Historian{pool: p}
	h.SetCapacityPolicy(90, config.FullPolicyStop)
	if err := h.enforceCapacity(context.Background()); err != nil {
		t.Fatalf("unevaluable should allow: %v", err)
	}

	// Query size error → allow.
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "1000")
	p.queryRowFn = func(string, []any) pgx.Row { return rowErr(errors.New("size")) }
	if err := h.enforceCapacity(context.Background()); err != nil {
		t.Fatalf("size err should allow: %v", err)
	}
}

func TestEnforceCapacity_DropOldest(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "1000")
	sizes := []int64{2000, 2000, 500} // over, still over during drop, then under
	var sizeIdx int
	oldest := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newest := oldest.Add(48 * time.Hour)
	p := &fakePool{
		queryRowFn: func(sql string, _ []any) pgx.Row {
			switch {
			case sqlHas(sql, "pg_database_size"):
				v := sizes[sizeIdx]
				if sizeIdx < len(sizes)-1 {
					sizeIdx++
				}
				return rowInt64(v)
			case sqlHas(sql, "timescaledb_information.chunks"):
				return rowErr(errors.New("no chunks view"))
			case sqlHas(sql, "min(time)", "max(time)"):
				return rowTimes(oldest, newest)
			default:
				return rowErr(errors.New(sql))
			}
		},
		execFn: func(sql string, _ []any) (pgconn.CommandTag, error) {
			if sqlHas(sql, "drop_chunks") {
				return pgconn.NewCommandTag(""), errors.New("no drop_chunks")
			}
			if sqlHas(sql, "delete from collector.samples") {
				return pgconn.NewCommandTag("DELETE 10"), nil
			}
			return pgconn.NewCommandTag(""), errors.New(sql)
		},
	}
	h := &Historian{pool: p}
	h.SetCapacityPolicy(90, config.FullPolicyDropOldest)
	if err := h.enforceCapacity(context.Background()); err != nil {
		t.Fatalf("drop_oldest: %v", err)
	}

	// Still over after drop → halt.
	sizeIdx = 0
	sizes = []int64{2000, 2000, 2000, 2000}
	p.queryRowFn = func(sql string, _ []any) pgx.Row {
		switch {
		case sqlHas(sql, "pg_database_size"):
			v := sizes[0]
			return rowInt64(v)
		case sqlHas(sql, "timescaledb_information.chunks"):
			return rowErr(errors.New("no chunks"))
		case sqlHas(sql, "min(time)"):
			return rowTimes(oldest, newest)
		default:
			return rowErr(errors.New(sql))
		}
	}
	if err := h.enforceCapacity(context.Background()); !errors.Is(err, ErrCapacityHalt) {
		t.Fatalf("want halt still over, got %v", err)
	}

	// dropOldestChunk failure bubbles as halt.
	p.execFn = func(sql string, _ []any) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag(""), errors.New("cannot delete")
	}
	p.queryRowFn = func(sql string, _ []any) pgx.Row {
		switch {
		case sqlHas(sql, "pg_database_size"):
			return rowInt64(2000)
		case sqlHas(sql, "timescaledb_information.chunks"):
			return rowErr(errors.New("no"))
		case sqlHas(sql, "min(time)"):
			return rowTimes(oldest, newest)
		default:
			return rowErr(errors.New(sql))
		}
	}
	if err := h.enforceCapacity(context.Background()); !errors.Is(err, ErrCapacityHalt) {
		t.Fatalf("want halt on drop fail, got %v", err)
	}
}

func TestDropOldestChunk_Branches(t *testing.T) {
	cutoff := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	p := &fakePool{
		queryRowFn: func(sql string, _ []any) pgx.Row {
			if sqlHas(sql, "timescaledb_information.chunks") {
				return rowTimePtr(&cutoff)
			}
			return rowErr(errors.New(sql))
		},
		execFn: func(sql string, _ []any) (pgconn.CommandTag, error) {
			if sqlHas(sql, "drop_chunks") {
				return pgconn.NewCommandTag("SELECT 0"), nil // RowsAffected 0 → n=1
			}
			return pgconn.NewCommandTag(""), errors.New(sql)
		},
	}
	n, err := (&Historian{pool: p}).dropOldestChunk(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}

	// No samples to drop.
	p.queryRowFn = func(sql string, _ []any) pgx.Row {
		if sqlHas(sql, "timescaledb_information.chunks") {
			return rowErr(errors.New("no"))
		}
		if sqlHas(sql, "min(time)") {
			return fakeRow{scan: func(dest ...any) error {
				*(dest[0].(**time.Time)) = nil
				*(dest[1].(**time.Time)) = nil
				return nil
			}}
		}
		return rowErr(errors.New(sql))
	}
	if _, err := (&Historian{pool: p}).dropOldestChunk(context.Background()); err == nil {
		t.Fatal("expected no samples")
	}

	// Short span → hour cut; delete removes 0 rows.
	oldest := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	newest := oldest.Add(2 * time.Hour)
	p.queryRowFn = func(sql string, _ []any) pgx.Row {
		if sqlHas(sql, "timescaledb_information.chunks") {
			return rowErr(errors.New("no"))
		}
		if sqlHas(sql, "min(time)") {
			return rowTimes(oldest, newest)
		}
		return rowErr(errors.New(sql))
	}
	p.execFn = func(sql string, _ []any) (pgconn.CommandTag, error) {
		if sqlHas(sql, "drop_chunks") {
			return pgconn.NewCommandTag(""), errors.New("no")
		}
		return pgconn.NewCommandTag("DELETE 0"), nil
	}
	if _, err := (&Historian{pool: p}).dropOldestChunk(context.Background()); err == nil {
		t.Fatal("expected delete 0 error")
	}

	// MIN/MAX query error.
	p.queryRowFn = func(sql string, _ []any) pgx.Row {
		return rowErr(errors.New("boom"))
	}
	if _, err := (&Historian{pool: p}).dropOldestChunk(context.Background()); err == nil {
		t.Fatal("expected min/max error")
	}
}

func TestDropOldestUntilUnder_NoProgress(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "1000")
	oldest := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newest := oldest.Add(time.Hour)
	p := &fakePool{
		queryRowFn: func(sql string, _ []any) pgx.Row {
			switch {
			case sqlHas(sql, "pg_database_size"):
				return rowInt64(5000)
			case sqlHas(sql, "timescaledb_information.chunks"):
				return rowErr(errors.New("no"))
			case sqlHas(sql, "min(time)"):
				return rowTimes(oldest, newest)
			default:
				return rowErr(errors.New(sql))
			}
		},
		execFn: func(sql string, _ []any) (pgconn.CommandTag, error) {
			if sqlHas(sql, "drop_chunks") {
				return pgconn.NewCommandTag(""), errors.New("no")
			}
			// Pretend delete works but size never shrinks → dropOldestChunk returns 1,
			// then next round still over; eventually max rounds or we force 0 drop.
			return pgconn.NewCommandTag("DELETE 1"), nil
		},
	}
	h := &Historian{pool: p}
	// Force no-progress by making delete report 0 after first path uses drop that fails size check.
	p.execFn = func(sql string, _ []any) (pgconn.CommandTag, error) {
		if sqlHas(sql, "delete") {
			return pgconn.NewCommandTag("DELETE 0"), nil
		}
		return pgconn.NewCommandTag(""), errors.New("no drop")
	}
	if err := h.dropOldestUntilUnder(context.Background(), 1000, 90); err == nil {
		t.Fatal("expected no older chunks/rows")
	}

	p.queryRowFn = func(sql string, _ []any) pgx.Row {
		if sqlHas(sql, "pg_database_size") {
			return rowErr(errors.New("size"))
		}
		return rowErr(errors.New(sql))
	}
	if err := h.dropOldestUntilUnder(context.Background(), 1000, 90); err == nil {
		t.Fatal("expected policyLimitBytes error")
	}
}

func TestResolveFreeBytes_PercentClampAndUnavailable(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "5000")
	free, source, _, _, _, _, limit := resolveFreeBytes(1000, 0) // → 90 clamp but env wins
	if source != "env_limit" || free == nil || *free != 4000 || limit == nil || *limit != 5000 {
		t.Fatalf("env free=%v source=%s limit=%v", free, source, limit)
	}

	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "not-a-number")
	t.Setenv("LEVEL2_DB_DATA_PATH", `\\?\Level2Missing\path`)
	free, source, _, _, _, _, _ = resolveFreeBytes(100, 200)
	if source != "unavailable" || free != nil {
		t.Fatalf("want unavailable, got free=%v source=%s", free, source)
	}
}

func TestResolveCapacityLimit_Unavailable(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "")
	t.Setenv("LEVEL2_DB_DATA_PATH", `\\?\Level2Missing\path`)
	limit, total := resolveCapacityLimit(50)
	if limit != 0 || total != 0 {
		t.Fatalf("limit=%d total=%d", limit, total)
	}
}

func TestDiskSpace_InvalidPath(t *testing.T) {
	_, _, err := diskSpace(`\\?\`)
	if err == nil {
		t.Fatal("expected diskSpace error for invalid path")
	}
}

func TestDropOldestUntilUnder_AlreadyUnder(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "1000")
	p := &fakePool{
		queryRowFn: func(sql string, _ []any) pgx.Row {
			if sqlHas(sql, "pg_database_size") {
				return rowInt64(100)
			}
			return rowErr(errors.New(sql))
		},
	}
	if err := (&Historian{pool: p}).dropOldestUntilUnder(context.Background(), 1000, 90); err != nil {
		t.Fatal(err)
	}
}

func TestDropOldestChunk_DropChunksFallbackAndShortCut(t *testing.T) {
	// drop_chunks succeeds after min/max (chunk metadata missing).
	oldest := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	newest := oldest.Add(20 * time.Hour)
	p := &fakePool{
		queryRowFn: func(sql string, _ []any) pgx.Row {
			if sqlHas(sql, "timescaledb_information.chunks") {
				return rowErr(errors.New("no meta"))
			}
			if sqlHas(sql, "min(time)") {
				return rowTimes(oldest, newest)
			}
			return rowErr(errors.New(sql))
		},
		execFn: func(sql string, _ []any) (pgconn.CommandTag, error) {
			if sqlHas(sql, "drop_chunks") {
				return pgconn.NewCommandTag("SELECT 3"), nil
			}
			return pgconn.NewCommandTag(""), errors.New(sql)
		},
	}
	n, err := (&Historian{pool: p}).dropOldestChunk(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}

	// Equal oldest/newest → no samples.
	p.queryRowFn = func(sql string, _ []any) pgx.Row {
		if sqlHas(sql, "timescaledb_information.chunks") {
			return rowErr(errors.New("no"))
		}
		if sqlHas(sql, "min(time)") {
			return rowTimes(oldest, oldest)
		}
		return rowErr(errors.New(sql))
	}
	if _, err := (&Historian{pool: p}).dropOldestChunk(context.Background()); err == nil {
		t.Fatal("expected no samples for equal bounds")
	}

	// Very short span forces minute cut; delete succeeds.
	newest2 := oldest.Add(30 * time.Second)
	p.queryRowFn = func(sql string, _ []any) pgx.Row {
		if sqlHas(sql, "timescaledb_information.chunks") {
			return rowErr(errors.New("no"))
		}
		if sqlHas(sql, "min(time)") {
			return rowTimes(oldest, newest2)
		}
		return rowErr(errors.New(sql))
	}
	p.execFn = func(sql string, _ []any) (pgconn.CommandTag, error) {
		if sqlHas(sql, "drop_chunks") {
			return pgconn.NewCommandTag(""), errors.New("no")
		}
		if sqlHas(sql, "delete") {
			return pgconn.NewCommandTag("DELETE 2"), nil
		}
		return pgconn.NewCommandTag(""), errors.New(sql)
	}
	n, err = (&Historian{pool: p}).dropOldestChunk(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("short span n=%d err=%v", n, err)
	}
}

func TestResolveFreeBytes_DiskAvailCapped(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "")
	dir := t.TempDir()
	t.Setenv("LEVEL2_DB_DATA_PATH", dir)
	// used near 0 so under_limit is huge; source becomes disk_avail when avail is smaller.
	free, source, _, diskTotal, diskAvail, _, limit := resolveFreeBytes(0, 100)
	if diskTotal == nil || diskAvail == nil || limit == nil || free == nil {
		t.Skipf("statfs unavailable for %s", dir)
	}
	if *free > *diskAvail {
		t.Fatalf("free %d > avail %d", *free, *diskAvail)
	}
	if source != "under_limit" && source != "disk_avail" {
		t.Fatalf("source=%s", source)
	}
}

func TestMaskDatabaseURL_UserOnlyAndInlineFallback(t *testing.T) {
	got := MaskDatabaseURL("postgres://onlyuser@host:5432/db")
	if !strings.Contains(got, "onlyuser") || strings.Contains(got, "***") {
		t.Fatalf("%q", got)
	}
	// Weird parse that still has scheme+user:pass@host should mask via inline helper path.
	got = MaskDatabaseURL("postgres://u:p@")
	if got == "" {
		t.Fatal("empty")
	}
}

func TestPool_NonPoolFake(t *testing.T) {
	h := &Historian{pool: &fakePool{}}
	if h.Pool() != nil {
		t.Fatal("fake pool should not assert to *pgxpool.Pool")
	}
}

func TestWriteBatch_CapacityHalt(t *testing.T) {
	t.Setenv("LEVEL2_DB_CAPACITY_BYTES", "100")
	p := &fakePool{
		queryRowFn: func(sql string, _ []any) pgx.Row {
			return rowInt64(500)
		},
	}
	h := &Historian{pool: p}
	h.SetCapacityPolicy(90, config.FullPolicyStop)
	err := h.WriteBatch(context.Background(), []core.Sample{{TagID: "x"}})
	if !errors.Is(err, ErrCapacityHalt) {
		t.Fatalf("err=%v", err)
	}
}
