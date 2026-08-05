package timescale

import (
	"context"
	"time"

	"github.com/popelev/level2/internal/core"
)

// QueryHistory returns samples for tag_id in [from, to], newest last, limited.
func (h *Historian) QueryHistory(ctx context.Context, tagID string, from, to time.Time, limit int) ([]core.Sample, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-1 * time.Hour)
	}
	rows, err := h.pool.Query(ctx, `
SELECT time, tag_id, value_num, value_text, value_bool, quality
FROM collector.samples
WHERE tag_id = $1 AND time >= $2 AND time <= $3
ORDER BY time ASC
LIMIT $4`, tagID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]core.Sample, 0, 128)
	for rows.Next() {
		var s core.Sample
		var q int
		if err := rows.Scan(&s.Time, &s.TagID, &s.ValueNum, &s.ValueText, &s.ValueBool, &q); err != nil {
			return nil, err
		}
		s.Quality = core.Quality(q)
		out = append(out, s)
	}
	return out, rows.Err()
}
