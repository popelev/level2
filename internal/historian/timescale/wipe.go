package timescale

import (
	"context"
	"fmt"
	"time"
)

// WipeResult describes how historian samples were cleared.
type WipeResult struct {
	Method     string `json:"method"` // truncate | delete
	ApproxRows int64  `json:"approx_rows_before"`
}

// WipeSamples removes all rows from collector.samples efficiently (TRUNCATE,
// with DELETE fallback). Intended for lab reset — not a selective retention tool.
func (h *Historian) WipeSamples(ctx context.Context) (WipeResult, error) {
	var out WipeResult
	if h == nil || h.pool == nil {
		return out, fmt.Errorf("historian not configured")
	}
	qCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	if err := h.pool.QueryRow(qCtx, `SELECT approximate_row_count('collector.samples')`).Scan(&out.ApproxRows); err != nil {
		_ = h.pool.QueryRow(qCtx, `
SELECT COALESCE(
  (SELECT GREATEST(c.reltuples::bigint, 0)
   FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
   WHERE n.nspname = 'collector' AND c.relname = 'samples'),
  0)`).Scan(&out.ApproxRows)
	}
	if out.ApproxRows < 0 {
		out.ApproxRows = 0
	}

	if _, err := h.pool.Exec(qCtx, `TRUNCATE TABLE collector.samples`); err == nil {
		out.Method = "truncate"
		return out, nil
	} else {
		// Fallback when TRUNCATE is blocked (permissions / older Timescale quirks).
		tag, delErr := h.pool.Exec(qCtx, `DELETE FROM collector.samples`)
		if delErr != nil {
			return out, fmt.Errorf("wipe samples: truncate: %v; delete: %w", err, delErr)
		}
		out.Method = "delete"
		if tag.RowsAffected() > 0 {
			out.ApproxRows = tag.RowsAffected()
		}
		return out, nil
	}
}
