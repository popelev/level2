package store

import (
	"sync"
	"time"

	"github.com/popelev/level2/internal/core"
)

// Live holds the latest sample per tag_id for API/WS.
type Live struct {
	mu   sync.RWMutex
	byID map[string]core.Sample
}

func NewLive() *Live {
	return &Live{byID: make(map[string]core.Sample)}
}

func (l *Live) Update(s core.Sample) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.byID[s.TagID] = s
}

func (l *Live) Get(tagID string) (core.Sample, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	s, ok := l.byID[tagID]
	return s, ok
}

func (l *Live) All() []core.Sample {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]core.Sample, 0, len(l.byID))
	for _, s := range l.byID {
		out = append(out, s)
	}
	return out
}

// SnapshotTags returns configured tags merged with live values.
func (l *Live) SnapshotTags(tags []core.Tag) []TagValue {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]TagValue, 0, len(tags))
	for _, t := range tags {
		tv := TagValue{Tag: t}
		if s, ok := l.byID[t.ID]; ok {
			tv.Sample = &s
			tv.UpdatedAt = s.Time
		}
		out = append(out, tv)
	}
	return out
}

type TagValue struct {
	Tag       core.Tag     `json:"tag"`
	Sample    *core.Sample `json:"sample,omitempty"`
	UpdatedAt time.Time    `json:"updated_at,omitempty"`
}
