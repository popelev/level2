package store

import (
	"sync"
	"time"

	"github.com/popelev/level2/internal/core"
)

const maxPollIntervals = 5

type liveEntry struct {
	sample    core.Sample
	lastRecv  time.Time
	intervals []int64 // ms between last receives (newest last), max 5
}

// Live holds the latest sample per tag_id for API/WS.
type Live struct {
	mu   sync.RWMutex
	byID map[string]*liveEntry
}

func NewLive() *Live {
	return &Live{byID: make(map[string]*liveEntry)}
}

func (l *Live) Update(s core.Sample) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	ent, ok := l.byID[s.TagID]
	if !ok {
		l.byID[s.TagID] = &liveEntry{sample: s, lastRecv: now}
		return
	}
	if !ent.lastRecv.IsZero() {
		ms := now.Sub(ent.lastRecv).Milliseconds()
		if ms > 0 {
			ent.intervals = append(ent.intervals, ms)
			if len(ent.intervals) > maxPollIntervals {
				ent.intervals = append([]int64(nil), ent.intervals[len(ent.intervals)-maxPollIntervals:]...)
			}
		}
	}
	ent.sample = s
	ent.lastRecv = now
}

func (l *Live) Get(tagID string) (core.Sample, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	ent, ok := l.byID[tagID]
	if !ok {
		return core.Sample{}, false
	}
	return ent.sample, true
}

func (l *Live) All() []core.Sample {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]core.Sample, 0, len(l.byID))
	for _, ent := range l.byID {
		out = append(out, ent.sample)
	}
	return out
}

// MarkQuality sets quality on existing samples (keeps last values/timestamps payload).
// Returns the updated samples (for FanIn/WS). Tags with no live entry are skipped.
func (l *Live) MarkQuality(tagIDs []string, q core.Quality) []core.Sample {
	if l == nil || len(tagIDs) == 0 {
		return nil
	}
	now := time.Now().UTC()
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]core.Sample, 0, len(tagIDs))
	for _, id := range tagIDs {
		ent, ok := l.byID[id]
		if !ok {
			continue
		}
		if ent.sample.Quality == q {
			continue
		}
		ent.sample.Quality = q
		ent.sample.Time = now
		out = append(out, ent.sample)
	}
	return out
}

// Clear removes every cached sample. Used after historian wipe so FanIn treats
// the next poll as a first sample (no Phase 1 suppress against stale Live).
func (l *Live) Clear() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	n := len(l.byID)
	l.byID = make(map[string]*liveEntry)
	return n
}

// SnapshotTags returns configured tags merged with live values.
func (l *Live) SnapshotTags(tags []core.Tag) []TagValue {
	return l.SnapshotDevices([]core.Device{{Tags: tags}})
}

// SnapshotDevices flattens device tags and attaches live samples + device_id.
func (l *Live) SnapshotDevices(devices []core.Device) []TagValue {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]TagValue, 0)
	for _, d := range devices {
		for _, t := range d.Tags {
			tv := TagValue{DeviceID: d.ID, Tag: t}
			if ent, ok := l.byID[t.ID]; ok {
				cp := ent.sample
				tv.Sample = &cp
				ts := ent.sample.Time
				tv.UpdatedAt = &ts
				if avg := avgIntervals(ent.intervals); avg > 0 {
					tv.PollAvgMs = &avg
				}
			}
			out = append(out, tv)
		}
	}
	return out
}

func avgIntervals(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	var sum int64
	for _, v := range xs {
		sum += v
	}
	return sum / int64(len(xs))
}

type TagValue struct {
	DeviceID  string       `json:"device_id"`
	Tag       core.Tag     `json:"tag"`
	Sample    *core.Sample `json:"sample,omitempty"`
	UpdatedAt *time.Time   `json:"updated_at,omitempty"`
	// PollAvgMs is the average wall-clock gap between the last ≤5 samples received by the collector.
	PollAvgMs *int64 `json:"poll_avg_ms,omitempty"`
}
