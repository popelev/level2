package diag

import (
	"sync"
	"time"
)

// Incident kinds recorded for Overview / status summary (last-hour windows).
const (
	IncidentOPCDisconnect = "opc_disconnect"
	IncidentCollectorDown = "collector_not_ready"
	IncidentDBWriteError  = "db_write_error"
)

type incident struct {
	At       time.Time
	Kind     string
	DeviceID string
}

// IncidentTracker is a ring of downtime / failure events with timestamps.
type IncidentTracker struct {
	mu     sync.Mutex
	events []incident
	max    int
	window time.Duration
}

// NewIncidentTracker keeps roughly window-sized history (default 1h, cap 4096).
func NewIncidentTracker(max int, window time.Duration) *IncidentTracker {
	if max <= 0 {
		max = 4096
	}
	if window <= 0 {
		window = time.Hour
	}
	return &IncidentTracker{max: max, window: window}
}

var defaultIncidents = NewIncidentTracker(4096, time.Hour)

// SetDefaultIncidents registers the tracker used by Record* helpers.
func SetDefaultIncidents(t *IncidentTracker) {
	if t == nil {
		return
	}
	defaultIncidents = t
}

// DefaultIncidents returns the process-wide tracker.
func DefaultIncidents() *IncidentTracker {
	return defaultIncidents
}

// Clear drops all recorded incidents (Overview “drops / last hour” counters).
func (t *IncidentTracker) Clear() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.events = nil
	t.mu.Unlock()
}

// Record appends an incident (UTC now). Empty kind is ignored.
func (t *IncidentTracker) Record(kind, deviceID string) {
	if t == nil || kind == "" {
		return
	}
	now := time.Now().UTC()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(now.Add(-t.window))
	e := incident{At: now, Kind: kind, DeviceID: deviceID}
	if len(t.events) >= t.max {
		copy(t.events, t.events[1:])
		t.events[len(t.events)-1] = e
		return
	}
	t.events = append(t.events, e)
}

func (t *IncidentTracker) pruneLocked(before time.Time) {
	i := 0
	for i < len(t.events) && t.events[i].At.Before(before) {
		i++
	}
	if i > 0 {
		t.events = append([]incident(nil), t.events[i:]...)
	}
}

// Count returns events of kind within the trailing window (0 = tracker default).
func (t *IncidentTracker) Count(kind string, window time.Duration) int {
	if t == nil {
		return 0
	}
	if window <= 0 {
		window = t.window
	}
	cutoff := time.Now().UTC().Add(-window)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(cutoff)
	n := 0
	for _, e := range t.events {
		if e.Kind == kind && !e.At.Before(cutoff) {
			n++
		}
	}
	return n
}

// CountByDevice returns per-device counts for kind within the trailing window.
func (t *IncidentTracker) CountByDevice(kind string, window time.Duration) map[string]int {
	out := map[string]int{}
	if t == nil {
		return out
	}
	if window <= 0 {
		window = t.window
	}
	cutoff := time.Now().UTC().Add(-window)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(cutoff)
	for _, e := range t.events {
		if e.Kind != kind || e.At.Before(cutoff) {
			continue
		}
		id := e.DeviceID
		if id == "" {
			id = "_"
		}
		out[id]++
	}
	return out
}

// RecordOPCDisconnect notes connected → disconnected for a device.
func RecordOPCDisconnect(deviceID string) {
	defaultIncidents.Record(IncidentOPCDisconnect, deviceID)
}

// RecordCollectorDown notes /readyz becoming not-ready.
func RecordCollectorDown() {
	defaultIncidents.Record(IncidentCollectorDown, "")
}

// RecordDBWriteError notes a historian write failure.
func RecordDBWriteError() {
	defaultIncidents.Record(IncidentDBWriteError, "")
}
