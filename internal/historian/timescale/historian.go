package timescale

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
)

// Historian writes samples to Timescale/Postgres.
type Historian struct {
	pool             *pgxpool.Pool
	capacityPercent  atomic.Int64
	fullPolicy       atomic.Value // string
}

func New(ctx context.Context, databaseURL string) (*Historian, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	h := &Historian{pool: pool}
	h.SetCapacityPolicy(90, config.FullPolicyStop)
	return h, nil
}

func (h *Historian) Close(ctx context.Context) error {
	h.pool.Close()
	return nil
}

// Ping checks that the pool can reach Postgres (lightweight readiness probe).
func (h *Historian) Ping(ctx context.Context) error {
	if h == nil || h.pool == nil {
		return fmt.Errorf("historian not configured")
	}
	return h.pool.Ping(ctx)
}

func (h *Historian) EnsureSchema(ctx context.Context) error {
	sql := `
CREATE SCHEMA IF NOT EXISTS collector;
CREATE TABLE IF NOT EXISTS collector.samples (
  time        TIMESTAMPTZ NOT NULL,
  tag_id      TEXT NOT NULL,
  value_num   DOUBLE PRECISION,
  value_text  TEXT,
  value_bool  BOOLEAN,
  quality     INT NOT NULL,
  PRIMARY KEY (time, tag_id)
);
`
	if _, err := h.pool.Exec(ctx, sql); err != nil {
		return err
	}
	// Best-effort hypertable (Timescale). Ignore if extension missing.
	_, _ = h.pool.Exec(ctx, `SELECT create_hypertable('collector.samples', 'time', if_not_exists => TRUE)`)
	return nil
}

func (h *Historian) WriteBatch(ctx context.Context, samples []core.Sample) error {
	if len(samples) == 0 {
		return nil
	}
	if err := h.enforceCapacity(ctx); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString(`INSERT INTO collector.samples (time, tag_id, value_num, value_text, value_bool, quality) VALUES `)
	args := make([]any, 0, len(samples)*6)
	for i, s := range samples {
		if i > 0 {
			b.WriteByte(',')
		}
		base := i*6 + 1
		fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d)", base, base+1, base+2, base+3, base+4, base+5)
		args = append(args, s.Time, s.TagID, s.ValueNum, s.ValueText, s.ValueBool, int(s.Quality))
	}
	_, err := h.pool.Exec(ctx, b.String(), args...)
	return err
}
