package timescale

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ConnectionStatus is safe connection metadata (no secrets).
type ConnectionStatus struct {
	Connected      bool   `json:"connected"`
	PingOK         bool   `json:"ping_ok"`
	PingError      string `json:"ping_error,omitempty"`
	URLMasked      string `json:"url_masked"`
	Host           string `json:"host,omitempty"`
	Port           string `json:"port,omitempty"`
	Database       string `json:"database,omitempty"`
	User           string `json:"user,omitempty"`
	SSLMode        string `json:"sslmode,omitempty"`
	ServerVersion  string `json:"server_version,omitempty"`
	TimescaleVer   string `json:"timescale_version,omitempty"`
	PoolMaxConns   int32  `json:"pool_max_conns"`
	PoolTotalConns int32  `json:"pool_total_conns"`
	PoolIdleConns  int32  `json:"pool_idle_conns"`
	PoolAcquired   int32  `json:"pool_acquired_conns"`
}

// CapacityStats describes DB growth and remaining room for ETA.
type CapacityStats struct {
	DatabaseSizeBytes int64   `json:"database_size_bytes"`
	SamplesSizeBytes  int64   `json:"samples_size_bytes"`
	SamplesApproxRows int64   `json:"samples_approx_rows"`
	SamplesLast5Min   int64   `json:"samples_last_5min"`
	SamplesPerSec     float64 `json:"samples_per_sec"`
	AvgSampleBytes    float64 `json:"avg_sample_bytes"`
	GrowthBytesPerSec float64 `json:"growth_bytes_per_sec"`
	FreeBytes         *int64  `json:"free_bytes"`
	FreeBytesSource   string  `json:"free_bytes_source"`
	CapacityBytes     *int64  `json:"capacity_bytes,omitempty"`
	ETASeconds        *float64 `json:"eta_seconds"`
	WindowSeconds     int     `json:"window_seconds"`
}

// Status returns connection health and masked DSN fields.
func (h *Historian) Status(ctx context.Context, databaseURL string) ConnectionStatus {
	st := ConnectionStatus{
		URLMasked: MaskDatabaseURL(databaseURL),
	}
	host, port, db, user, ssl := ParseDatabaseURL(databaseURL)
	st.Host, st.Port, st.Database, st.User, st.SSLMode = host, port, db, user, ssl

	if h == nil || h.pool == nil {
		st.PingError = "historian not configured"
		return st
	}

	stat := h.pool.Stat()
	st.PoolMaxConns = stat.MaxConns()
	st.PoolTotalConns = stat.TotalConns()
	st.PoolIdleConns = stat.IdleConns()
	st.PoolAcquired = stat.AcquiredConns()

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := h.pool.Ping(pingCtx); err != nil {
		st.PingError = err.Error()
		return st
	}
	st.PingOK = true
	st.Connected = true

	_ = h.pool.QueryRow(ctx, `SHOW server_version`).Scan(&st.ServerVersion)
	_ = h.pool.QueryRow(ctx, `
SELECT COALESCE(extversion, '') FROM pg_extension WHERE extname = 'timescaledb'`).Scan(&st.TimescaleVer)
	return st
}

// Capacity queries size / recent write rate and estimates ETA when free space is known.
func (h *Historian) Capacity(ctx context.Context) (CapacityStats, error) {
	out := CapacityStats{WindowSeconds: 300, FreeBytesSource: "unavailable"}
	if h == nil || h.pool == nil {
		return out, fmt.Errorf("historian not configured")
	}

	qCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	if err := h.pool.QueryRow(qCtx, `SELECT pg_database_size(current_database())`).Scan(&out.DatabaseSizeBytes); err != nil {
		return out, fmt.Errorf("database size: %w", err)
	}

	_ = h.pool.QueryRow(qCtx, `
SELECT COALESCE(
  (SELECT pg_total_relation_size(c.oid)
   FROM pg_class c
   JOIN pg_namespace n ON n.oid = c.relnamespace
   WHERE n.nspname = 'collector' AND c.relname = 'samples'),
  0)`).Scan(&out.SamplesSizeBytes)

	_ = h.pool.QueryRow(qCtx, `
SELECT COALESCE(
  (SELECT c.reltuples::bigint
   FROM pg_class c
   JOIN pg_namespace n ON n.oid = c.relnamespace
   WHERE n.nspname = 'collector' AND c.relname = 'samples'),
  0)`).Scan(&out.SamplesApproxRows)

	_ = h.pool.QueryRow(qCtx, `
SELECT COUNT(*) FROM collector.samples WHERE time > now() - interval '5 minutes'`).Scan(&out.SamplesLast5Min)

	out.SamplesPerSec = float64(out.SamplesLast5Min) / float64(out.WindowSeconds)
	if out.SamplesApproxRows > 0 && out.SamplesSizeBytes > 0 {
		out.AvgSampleBytes = float64(out.SamplesSizeBytes) / float64(out.SamplesApproxRows)
	} else {
		out.AvgSampleBytes = 120 // conservative row+index estimate
	}
	out.GrowthBytesPerSec = out.SamplesPerSec * out.AvgSampleBytes

	free, source, capacity := resolveFreeBytes(out.DatabaseSizeBytes)
	out.FreeBytesSource = source
	if capacity != nil {
		out.CapacityBytes = capacity
	}
	if free != nil {
		out.FreeBytes = free
		if out.GrowthBytesPerSec > 0 && *free >= 0 {
			eta := float64(*free) / out.GrowthBytesPerSec
			out.ETASeconds = &eta
		}
	}
	return out, nil
}

func resolveFreeBytes(usedBytes int64) (free *int64, source string, capacity *int64) {
	if raw := strings.TrimSpace(os.Getenv("LEVEL2_DB_CAPACITY_BYTES")); raw != "" {
		capN, err := strconv.ParseInt(raw, 10, 64)
		if err == nil && capN > 0 {
			capacity = &capN
			f := capN - usedBytes
			if f < 0 {
				f = 0
			}
			return &f, "env_limit", capacity
		}
	}
	return nil, "unavailable", nil
}

// MaskDatabaseURL redacts the password in a Postgres URL.
func MaskDatabaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return maskInlinePassword(raw)
	}
	if u.User != nil {
		user := u.User.Username()
		if _, has := u.User.Password(); has {
			u.User = url.UserPassword(user, "***")
		} else if user != "" {
			u.User = url.User(user)
		}
	}
	return u.String()
}

func maskInlinePassword(raw string) string {
	// postgres://user:pass@host — fallback if url.Parse fails oddly
	at := strings.LastIndex(raw, "@")
	scheme := strings.Index(raw, "://")
	if at < 0 || scheme < 0 || at < scheme {
		return raw
	}
	cred := raw[scheme+3 : at]
	if i := strings.Index(cred, ":"); i >= 0 {
		return raw[:scheme+3] + cred[:i] + ":***" + raw[at:]
	}
	return raw
}

// ParseDatabaseURL extracts non-secret connection parts.
func ParseDatabaseURL(raw string) (host, port, database, user, sslmode string) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", "", "", "", ""
	}
	host = u.Hostname()
	port = u.Port()
	if port == "" {
		port = "5432"
	}
	database = strings.TrimPrefix(u.Path, "/")
	if i := strings.Index(database, "/"); i >= 0 {
		database = database[:i]
	}
	if u.User != nil {
		user = u.User.Username()
	}
	sslmode = u.Query().Get("sslmode")
	return host, port, database, user, sslmode
}

// Pool exposes the underlying pool for tests.
func (h *Historian) Pool() *pgxpool.Pool {
	if h == nil {
		return nil
	}
	return h.pool
}
