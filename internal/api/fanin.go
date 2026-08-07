package api

import (
	"context"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/metrics"
	"github.com/popelev/level2/internal/store"
)

// FanIn updates live store and broadcasts WS from sample stream.
// Historian (out) receives a sample only when value or quality changed vs Live
// (or the tag has no previous sample). Live and WS update on every sample.
func FanIn(ctx context.Context, in <-chan core.Sample, live *store.Live, hub *Hub, out chan<- core.Sample) {
	for {
		select {
		case <-ctx.Done():
			return
		case s, ok := <-in:
			if !ok {
				return
			}
			prev, hasPrev := live.Get(s.TagID)
			// Bad/uncertain without a new value (link loss, skip): keep last value, flip quality only.
			if hasPrev && s.Quality != core.QualityGood &&
				s.ValueNum == nil && s.ValueText == nil && s.ValueBool == nil {
				merged := prev
				merged.Time = s.Time
				merged.Quality = s.Quality
				s = merged
			}
			unchanged := hasPrev && prev.SamePayload(s)
			live.Update(s)
			hub.Broadcast(s)
			if unchanged {
				metrics.SamplesSuppressedUnchanged.Inc()
				continue
			}
			select {
			case out <- s:
			case <-ctx.Done():
				return
			}
		}
	}
}
