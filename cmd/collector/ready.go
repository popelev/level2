package main

import (
	"context"
	"time"

	"github.com/popelev/level2/internal/diag"
)

// watchReady records collector becoming not-ready (mirrors /readyz).
func watchReady(ctx context.Context, ready func() bool) {
	if ready == nil {
		return
	}
	was := ready()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			now := ready()
			if was && !now {
				diag.RecordCollectorDown()
			}
			was = now
		}
	}
}
