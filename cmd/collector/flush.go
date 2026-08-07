package main

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	"github.com/popelev/level2/internal/historian/timescale"
	"github.com/popelev/level2/internal/metrics"
	"github.com/popelev/level2/internal/spool"
)

func flushLoop(ctx context.Context, log *slog.Logger, hist core.Historian, sp *spool.FileSpool, in <-chan core.Sample) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	buf := make([]core.Sample, 0, 256)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		batch := buf
		buf = make([]core.Sample, 0, 256)
		if err := hist.WriteBatch(ctx, batch); err != nil {
			if errors.Is(err, timescale.ErrCapacityHalt) {
				log.Warn("write batch halted by capacity policy", "err", err, "n", len(batch))
				diag.DBWrite(diag.LevelWarn, "historian halted by capacity policy", err.Error(), len(batch))
				return
			}
			metrics.WriteErrors.Inc()
			diag.RecordDBWriteError()
			log.Error("write batch", "err", err, "n", len(batch))
			diag.DBWrite(diag.LevelError, "historian write batch failed", err.Error(), len(batch))
			if serr := sp.Enqueue(batch); serr != nil {
				log.Error("spool enqueue", "err", serr)
				diag.DBWrite(diag.LevelError, "spool enqueue failed", serr.Error(), len(batch))
			} else {
				metrics.SamplesSpooled.Add(float64(len(batch)))
				metrics.SpoolDepth.Set(float64(sp.Len()))
				diag.DBWrite(diag.LevelWarn, "samples spooled to disk", "", len(batch))
			}
			return
		}
		metrics.SamplesWritten.Add(float64(len(batch)))
		diag.DBWrite(diag.LevelInfo, "wrote batch to timescale", "", len(batch))
	}
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case s := <-in:
			buf = append(buf, s)
			if len(buf) >= 200 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func replaySpool(ctx context.Context, log *slog.Logger, hist core.Historian, sp *spool.FileSpool) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			files, err := sp.List()
			if err != nil || len(files) == 0 {
				metrics.SpoolDepth.Set(0)
				continue
			}
			metrics.SpoolDepth.Set(float64(len(files)))
			path := files[0]
			for _, f := range files[1:] {
				if filepath.Base(f) < filepath.Base(path) {
					path = f
				}
			}
			batch, err := sp.Load(path)
			if err != nil {
				log.Warn("spool load", "path", path, "err", err)
				_ = sp.Remove(path)
				continue
			}
			if err := hist.WriteBatch(ctx, batch); err != nil {
				if errors.Is(err, timescale.ErrCapacityHalt) {
					log.Warn("spool replay halted by capacity policy", "err", err)
					diag.DBWrite(diag.LevelWarn, "spool replay halted by capacity policy", err.Error(), len(batch))
					continue
				}
				log.Warn("spool replay failed", "err", err)
				diag.RecordDBWriteError()
				diag.DBWrite(diag.LevelError, "spool replay write failed", err.Error(), len(batch))
				continue
			}
			metrics.SamplesWritten.Add(float64(len(batch)))
			_ = sp.Remove(path)
			metrics.SpoolDepth.Set(float64(sp.Len()))
			log.Info("spool replayed", "n", len(batch), "path", filepath.Base(path))
			diag.DBWrite(diag.LevelInfo, "spool replayed to timescale", filepath.Base(path), len(batch))
		}
	}
}
