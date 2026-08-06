package timescale

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/metrics"
)

// ErrCapacityHalt means writes were skipped by policy (do not spool).
var ErrCapacityHalt = errors.New("historian writes halted by capacity policy")

// CapacityPolicySettings is the live full-disk policy applied before WriteBatch.
type CapacityPolicySettings struct {
	Percent int
	Policy  string
}

// SetCapacityPolicy updates the live policy used by WriteBatch (thread-safe).
func (h *Historian) SetCapacityPolicy(percent int, policy string) {
	if h == nil {
		return
	}
	if percent < 1 {
		percent = 90
	}
	if percent > 100 {
		percent = 100
	}
	if policy == "" {
		policy = config.FullPolicyStop
	}
	h.capacityPercent.Store(int64(percent))
	h.fullPolicy.Store(policy)
}

// CapacityPolicy returns the live policy settings.
func (h *Historian) CapacityPolicy() CapacityPolicySettings {
	if h == nil {
		return CapacityPolicySettings{Percent: 90, Policy: config.FullPolicyStop}
	}
	p := int(h.capacityPercent.Load())
	if p < 1 {
		p = 90
	}
	pol, _ := h.fullPolicy.Load().(string)
	if pol == "" {
		pol = config.FullPolicyStop
	}
	return CapacityPolicySettings{Percent: p, Policy: pol}
}

// enforceCapacity applies the configured full-disk policy when used >= limit.
// Returns ErrCapacityHalt when the batch must be skipped (not spooled).
func (h *Historian) enforceCapacity(ctx context.Context) error {
	if h == nil || h.pool == nil {
		return nil
	}
	settings := h.CapacityPolicy()
	limit, used, _, err := h.policyLimitBytes(ctx, settings.Percent)
	if err != nil || limit <= 0 {
		return nil // cannot evaluate — allow write
	}
	if used < limit {
		return nil
	}

	switch settings.Policy {
	case config.FullPolicyDropOldest:
		if err := h.dropOldestUntilUnder(ctx, limit, settings.Percent); err != nil {
			slog.Warn("capacity drop_oldest failed", "err", err, "used", used, "limit", limit)
			metrics.CapacityHalts.Inc()
			return fmt.Errorf("%w: drop_oldest failed: %v", ErrCapacityHalt, err)
		}
		_, used2, _, err2 := h.policyLimitBytes(ctx, settings.Percent)
		if err2 == nil && used2 >= limit {
			metrics.CapacityHalts.Inc()
			slog.Warn("capacity still over limit after drop_oldest", "used", used2, "limit", limit)
			return fmt.Errorf("%w: still over limit after drop_oldest", ErrCapacityHalt)
		}
		return nil
	case config.FullPolicyRotate:
		metrics.CapacityHalts.Inc()
		slog.Warn("capacity rotate policy is Phase 2 stub; halting writes",
			"used", used, "limit", limit, "percent", settings.Percent)
		return fmt.Errorf("%w: rotate not implemented (Phase 2)", ErrCapacityHalt)
	case config.FullPolicyExpandLimit:
		metrics.CapacityHalts.Inc()
		slog.Warn("capacity expand_limit: raise capacity_percent to resume writes",
			"used", used, "limit", limit, "percent", settings.Percent)
		return fmt.Errorf("%w: raise capacity_percent (expand_limit)", ErrCapacityHalt)
	default: // stop
		metrics.CapacityHalts.Inc()
		slog.Warn("capacity stop: skipping WriteBatch",
			"used", used, "limit", limit, "percent", settings.Percent)
		return fmt.Errorf("%w: stop policy", ErrCapacityHalt)
	}
}

func (h *Historian) policyLimitBytes(ctx context.Context, percent int) (limit, used, diskTotal int64, err error) {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := h.pool.QueryRow(qCtx, `SELECT pg_database_size(current_database())`).Scan(&used); err != nil {
		return 0, 0, 0, err
	}
	limit, diskTotal = resolveCapacityLimit(percent)
	return limit, used, diskTotal, nil
}

// resolveCapacityLimit computes the byte ceiling from env override or Statfs * percent.
func resolveCapacityLimit(percent int) (limit, diskTotal int64) {
	if percent < 1 {
		percent = 90
	}
	if percent > 100 {
		percent = 100
	}

	if raw := strings.TrimSpace(os.Getenv("LEVEL2_DB_CAPACITY_BYTES")); raw != "" {
		if capN, err := strconv.ParseInt(raw, 10, 64); err == nil && capN > 0 {
			return capN, capN
		}
	}

	for _, path := range dbDiskPaths() {
		_, total, err := diskSpace(path)
		if err != nil || total <= 0 {
			continue
		}
		limit = total * int64(percent) / 100
		if limit < 1 {
			limit = 1
		}
		return limit, total
	}
	return 0, 0
}

func (h *Historian) dropOldestUntilUnder(ctx context.Context, limit int64, percent int) error {
	const maxRounds = 8
	for round := 0; round < maxRounds; round++ {
		_, used, _, err := h.policyLimitBytes(ctx, percent)
		if err != nil {
			return err
		}
		if used < limit {
			return nil
		}
		dropped, err := h.dropOldestChunk(ctx)
		if err != nil {
			return err
		}
		if dropped == 0 {
			return fmt.Errorf("no older chunks/rows to drop")
		}
		metrics.CapacityDrops.Add(float64(dropped))
	}
	return nil
}

// dropOldestChunk removes the oldest Timescale chunk, or deletes the oldest time slice.
func (h *Historian) dropOldestChunk(ctx context.Context) (int, error) {
	qCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var cutoff *time.Time
	err := h.pool.QueryRow(qCtx, `
SELECT MIN(range_start)::timestamptz
FROM timescaledb_information.chunks
WHERE hypertable_schema = 'collector' AND hypertable_name = 'samples'
  AND range_start > (
    SELECT MIN(range_start) FROM timescaledb_information.chunks
    WHERE hypertable_schema = 'collector' AND hypertable_name = 'samples'
  )`).Scan(&cutoff)
	if err == nil && cutoff != nil {
		tag, err := h.pool.Exec(qCtx, `SELECT drop_chunks('collector.samples', older_than => $1::timestamptz)`, *cutoff)
		if err == nil {
			n := int(tag.RowsAffected())
			if n <= 0 {
				n = 1
			}
			return n, nil
		}
	}

	var oldest, newest *time.Time
	if err := h.pool.QueryRow(qCtx, `
SELECT MIN(time), MAX(time) FROM collector.samples`).Scan(&oldest, &newest); err != nil {
		return 0, err
	}
	if oldest == nil || newest == nil || !newest.After(*oldest) {
		return 0, fmt.Errorf("no samples to drop")
	}
	span := newest.Sub(*oldest)
	cut := oldest.Add(span / 10)
	if span < 10*time.Hour {
		cut = oldest.Add(time.Hour)
	}
	if !cut.After(*oldest) {
		cut = oldest.Add(time.Minute)
	}
	if !cut.Before(*newest) {
		cut = newest.Add(-time.Second)
	}

	if _, err := h.pool.Exec(qCtx, `SELECT drop_chunks('collector.samples', older_than => $1::timestamptz)`, cut); err == nil {
		return 1, nil
	}

	tag, err := h.pool.Exec(qCtx, `DELETE FROM collector.samples WHERE time < $1`, cut)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 0 {
		return 0, fmt.Errorf("delete removed 0 rows")
	}
	return 1, nil
}
