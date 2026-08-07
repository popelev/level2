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

// ErrCapacityBusy means drop_oldest is reclaiming space; caller should spool
// and retry/drain when under limit again. Not a halt — data must not be dropped.
var ErrCapacityBusy = errors.New("historian capacity busy: spool while trimming")

// CapacityPolicySettings is the live full-disk policy applied before WriteBatch.
type CapacityPolicySettings struct {
	Percent int
	Policy  string
}

// Soft thresholds relative to the hard byte limit.
const (
	capacityApproachFrac   = 0.90 // start proactive trim
	capacityTargetFrac     = 0.85 // free down toward this fraction of limit
	capacityMaxChunksPass  = 64   // safety cap per drop_chunks call
	capacityMaxTrimPasses  = 4    // multi-chunk passes per WriteBatch
	capacityMaxDeleteRounds = 32  // DELETE / single-chunk fallback rounds
)

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

// enforceCapacity applies the configured full-disk policy.
// drop_oldest: proactive trim near the limit; if still over hard limit after a
// trim pass, returns ErrCapacityBusy so the collector spools (not halt).
// Halt only when nothing can be dropped and used is still over the hard limit.
func (h *Historian) enforceCapacity(ctx context.Context) error {
	if h == nil || h.pool == nil {
		return nil
	}
	settings := h.CapacityPolicy()
	limit, used, _, err := h.policyLimitBytes(ctx, settings.Percent)
	if err != nil || limit <= 0 {
		return nil // cannot evaluate — allow write
	}

	switch settings.Policy {
	case config.FullPolicyDropOldest:
		return h.enforceDropOldest(ctx, limit, used, settings.Percent)
	case config.FullPolicyRotate:
		if used < limit {
			return nil
		}
		metrics.CapacityHalts.Inc()
		slog.Warn("capacity rotate policy is Phase 2 stub; halting writes",
			"used", used, "limit", limit, "percent", settings.Percent)
		return fmt.Errorf("%w: rotate not implemented (Phase 2)", ErrCapacityHalt)
	case config.FullPolicyExpandLimit:
		if used < limit {
			return nil
		}
		metrics.CapacityHalts.Inc()
		slog.Warn("capacity expand_limit: raise capacity_percent to resume writes",
			"used", used, "limit", limit, "percent", settings.Percent)
		return fmt.Errorf("%w: raise capacity_percent (expand_limit)", ErrCapacityHalt)
	default: // stop
		if used < limit {
			return nil
		}
		metrics.CapacityHalts.Inc()
		slog.Warn("capacity stop: skipping WriteBatch",
			"used", used, "limit", limit, "percent", settings.Percent)
		return fmt.Errorf("%w: stop policy", ErrCapacityHalt)
	}
}

func (h *Historian) enforceDropOldest(ctx context.Context, limit, used int64, percent int) error {
	approach := int64(float64(limit) * capacityApproachFrac)
	if approach < 1 {
		approach = 1
	}
	target := int64(float64(limit) * capacityTargetFrac)
	if target < 1 {
		target = 1
	}
	if target >= limit {
		target = limit - 1
		if target < 1 {
			target = 1
		}
	}

	// Plenty of room — no trim, allow write.
	if used < approach {
		return nil
	}

	trimmed, estFreed, trimErr := h.dropOldestUntilUnder(ctx, target, percent)
	_, used2, _, err2 := h.policyLimitBytes(ctx, percent)
	if err2 != nil {
		// Size re-check failed after trim — allow write if we were only approaching.
		if used < limit {
			return nil
		}
		if trimmed > 0 || estFreed > 0 {
			slog.Warn("capacity size re-check failed after trim; spooling",
				"err", err2, "trimmed", trimmed, "est_freed", estFreed)
			return fmt.Errorf("%w: size re-check failed: %v", ErrCapacityBusy, err2)
		}
		metrics.CapacityHalts.Inc()
		return fmt.Errorf("%w: size re-check failed: %v", ErrCapacityHalt, err2)
	}

	if used2 < limit {
		if trimmed > 0 {
			slog.Info("capacity under limit after drop_oldest",
				"used", used2, "limit", limit, "target", target, "dropped", trimmed, "est_freed", estFreed)
		}
		return nil
	}

	// Still over hard limit.
	if trimErr != nil && trimmed == 0 && estFreed == 0 {
		metrics.CapacityHalts.Inc()
		slog.Warn("capacity drop_oldest exhausted; halting",
			"err", trimErr, "used", used2, "limit", limit)
		return fmt.Errorf("%w: drop_oldest failed: %v", ErrCapacityHalt, trimErr)
	}

	// Made progress or still reclaiming (pg size may lag / bloat) — spool, do not halt.
	slog.Warn("capacity over limit while trimming; spool writes",
		"used", used2, "limit", limit, "target", target, "dropped", trimmed, "est_freed", estFreed)
	return fmt.Errorf("%w: still over limit after drop_oldest", ErrCapacityBusy)
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

type chunkSizeRow struct {
	rangeStart time.Time
	sizeBytes  int64
}

// planChunkDrop chooses how many oldest chunks to drop and the older_than cutoff
// (range_start of the first retained chunk). Keeps at least one newest chunk.
// Pure helper for tests.
func planChunkDrop(chunks []chunkSizeRow, needBytes, dbUsed int64, maxDrop int) (nDrop int, cutoff time.Time, estFreed int64, ok bool) {
	if len(chunks) <= 1 || needBytes <= 0 || maxDrop < 1 {
		return 0, time.Time{}, 0, false
	}
	if maxDrop > len(chunks)-1 {
		maxDrop = len(chunks) - 1
	}

	avg := int64(0)
	known := 0
	for _, c := range chunks {
		if c.sizeBytes > 0 {
			avg += c.sizeBytes
			known++
		}
	}
	if known > 0 {
		avg = avg / int64(known)
	} else if dbUsed > 0 && len(chunks) > 0 {
		avg = dbUsed / int64(len(chunks))
	}
	if avg < 1 {
		avg = 1
	}

	for i := 0; i < maxDrop; i++ {
		sz := chunks[i].sizeBytes
		if sz <= 0 {
			sz = avg
		}
		estFreed += sz
		nDrop++
		cutoff = chunks[i+1].rangeStart
		if estFreed >= needBytes {
			return nDrop, cutoff, estFreed, true
		}
	}
	return nDrop, cutoff, estFreed, nDrop > 0
}

func (h *Historian) listChunkSizes(ctx context.Context) ([]chunkSizeRow, error) {
	qCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	rows, err := h.pool.Query(qCtx, `
SELECT range_start,
       COALESCE(pg_total_relation_size(format('%I.%I', chunk_schema, chunk_name)::regclass), 0)
FROM timescaledb_information.chunks
WHERE hypertable_schema = 'collector' AND hypertable_name = 'samples'
ORDER BY range_start ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []chunkSizeRow
	for rows.Next() {
		var c chunkSizeRow
		if err := rows.Scan(&c.rangeStart, &c.sizeBytes); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// dropOldestUntilUnder frees data until used < target (or estimated free covers the gap).
// Returns dropped chunk/slice count and estimated bytes freed.
func (h *Historian) dropOldestUntilUnder(ctx context.Context, target int64, percent int) (dropped int, estFreed int64, err error) {
	for pass := 0; pass < capacityMaxTrimPasses; pass++ {
		_, used, _, err := h.policyLimitBytes(ctx, percent)
		if err != nil {
			return dropped, estFreed, err
		}
		if used < target {
			return dropped, estFreed, nil
		}
		need := used - target
		if need < 1 {
			need = 1
		}

		chunks, listErr := h.listChunkSizes(ctx)
		if listErr == nil && len(chunks) > 1 {
			n, cutoff, freed, ok := planChunkDrop(chunks, need, used, capacityMaxChunksPass)
			if ok {
				qCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
				tag, execErr := h.pool.Exec(qCtx, `SELECT drop_chunks('collector.samples', older_than => $1::timestamptz)`, cutoff)
				cancel()
				if execErr == nil {
					nExec := int(tag.RowsAffected())
					if nExec <= 0 {
						nExec = n
					}
					dropped += nExec
					estFreed += freed
					metrics.CapacityDrops.Add(float64(nExec))
					if freed >= need {
						return dropped, estFreed, nil
					}
					continue
				}
				slog.Warn("capacity multi-chunk drop_chunks failed; falling back", "err", execErr)
			}
		}

		// Fallback: single oldest chunk / DELETE slice (legacy path).
		n, ferr := h.dropOldestChunk(ctx)
		if ferr != nil {
			return dropped, estFreed, ferr
		}
		if n == 0 {
			return dropped, estFreed, fmt.Errorf("no older chunks/rows to drop")
		}
		dropped += n
		metrics.CapacityDrops.Add(float64(n))
		// Unknown size — credit a conservative share of need so we keep trying
		// across passes without treating every fallback as "zero progress".
		estFreed += need / int64(capacityMaxTrimPasses)
		if estFreed < 1 {
			estFreed = 1
		}
	}

	// Extra DELETE rounds if still over and multi-chunk path unavailable.
	for round := 0; round < capacityMaxDeleteRounds; round++ {
		_, used, _, err := h.policyLimitBytes(ctx, percent)
		if err != nil {
			return dropped, estFreed, err
		}
		if used < target {
			return dropped, estFreed, nil
		}
		n, ferr := h.dropOldestChunk(ctx)
		if ferr != nil {
			return dropped, estFreed, ferr
		}
		if n == 0 {
			return dropped, estFreed, fmt.Errorf("no older chunks/rows to drop")
		}
		dropped += n
		metrics.CapacityDrops.Add(float64(n))
		estFreed += 1
	}
	return dropped, estFreed, nil
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
