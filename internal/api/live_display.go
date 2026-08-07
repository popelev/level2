package api

import (
	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/store"
)

// countStaleAsBad is true when Live Good must not be trusted as OPC health
// (no global/sim-browser master). Per-tag Simulate still keeps Live Good.
func (s *Server) countStaleAsBad() bool {
	if s == nil || s.DevHub == nil {
		return false
	}
	return !s.tagSimulationActive() && !s.simBrowserActive()
}

// snapshotDevicesForAPI returns Live tag values with honest quality for API/UI:
// disconnected device + non-simulated tag → QualityBad (values/timestamps kept).
// Also reconciles Live + WS so clients do not keep advertising stale Good.
func (s *Server) snapshotDevicesForAPI(devs []core.Device) []store.TagValue {
	if s == nil || s.Live == nil {
		return nil
	}
	s.reconcileStaleLiveQuality(devs)
	tvs := s.Live.SnapshotDevices(devs)
	s.applyStaleQualityOverride(tvs)
	return tvs
}

// reconcileStaleLiveQuality flips Live Good→Bad for disconnected non-sim tags and
// broadcasts WS. Idempotent. Values stay; historian is not written here (poll/reconnect
// or emitBadSamples handle that path).
func (s *Server) reconcileStaleLiveQuality(devs []core.Device) {
	if !s.countStaleAsBad() || s.Live == nil || len(devs) == 0 {
		return
	}
	conn := s.DevHub.Status()
	ids := make([]string, 0)
	for _, d := range devs {
		if conn[d.ID] {
			continue
		}
		for _, t := range d.Tags {
			if t.Simulate {
				continue
			}
			ids = append(ids, t.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	updated := s.Live.MarkQualityPreserveTime(ids, core.QualityBad)
	if s.Hub == nil {
		return
	}
	for _, sample := range updated {
		s.Hub.Broadcast(sample)
	}
}

// applyStaleQualityOverride remaps Good→Bad on response copies when OPC is down.
// Defense in depth if Live was not yet reconciled (or Hub Status raced).
func (s *Server) applyStaleQualityOverride(tvs []store.TagValue) {
	if !s.countStaleAsBad() || len(tvs) == 0 {
		return
	}
	conn := s.DevHub.Status()
	for i := range tvs {
		if conn[tvs[i].DeviceID] || tvs[i].Tag.Simulate || tvs[i].Sample == nil {
			continue
		}
		if tvs[i].Sample.Quality != core.QualityGood {
			continue
		}
		cp := *tvs[i].Sample
		cp.Quality = core.QualityBad
		tvs[i].Sample = &cp
	}
}

// displaySampleForTag returns Live sample with the same stale override as list tags.
func (s *Server) displaySampleForTag(tagID string) (core.Sample, bool) {
	if s == nil || s.Live == nil {
		return core.Sample{}, false
	}
	if s.Devices != nil {
		s.reconcileStaleLiveQuality(s.Devices())
	}
	sample, ok := s.Live.Get(tagID)
	if !ok {
		return core.Sample{}, false
	}
	if !s.countStaleAsBad() || s.Devices == nil {
		return sample, true
	}
	conn := s.DevHub.Status()
	for _, d := range s.Devices() {
		for _, t := range d.Tags {
			if t.ID != tagID {
				continue
			}
			if !conn[d.ID] && !t.Simulate && sample.Quality == core.QualityGood {
				sample.Quality = core.QualityBad
			}
			return sample, true
		}
	}
	return sample, true
}

// effectiveQuality applies status/Overview honesty rules for one TagValue with a sample.
func (s *Server) effectiveQuality(tv store.TagValue, conn map[string]bool) core.Quality {
	if tv.Sample == nil {
		return core.QualityBad
	}
	q := tv.Sample.Quality
	if s.countStaleAsBad() && !conn[tv.DeviceID] && !tv.Tag.Simulate {
		return core.QualityBad
	}
	return q
}
