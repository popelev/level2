package diag

import (
	"strings"
	"sync"
	"time"
)

const (
	CategoryOPCRead  = "opc_read"
	CategoryDBWrite  = "db_write"
	CategoryOPCWrite = "opc_write"
)

const (
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// Entry is one diagnostics log line for the admin UI.
type Entry struct {
	Time     time.Time `json:"time"`
	Level    string    `json:"level"`
	Category string    `json:"category"`
	Message  string    `json:"message"`
	DeviceID string    `json:"device_id,omitempty"`
	TagID    string    `json:"tag_id,omitempty"`
	Detail   string    `json:"detail,omitempty"`
	Count    int       `json:"count,omitempty"`
}

// Buffer is a fixed-size ring of recent diagnostic entries.
type Buffer struct {
	mu      sync.RWMutex
	entries []Entry
	max     int
}

func NewBuffer(max int) *Buffer {
	if max <= 0 {
		max = 2000
	}
	return &Buffer{max: max}
}

var defaultBuf *Buffer

// SetDefault registers the buffer used by Record helpers.
func SetDefault(b *Buffer) {
	defaultBuf = b
}

func (b *Buffer) Add(e Entry) {
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.entries) >= b.max {
		copy(b.entries, b.entries[1:])
		b.entries[len(b.entries)-1] = e
		return
	}
	b.entries = append(b.entries, e)
}

func (b *Buffer) Clear() {
	b.mu.Lock()
	b.entries = nil
	b.mu.Unlock()
}

// NormalizeCategory maps UI/API aliases onto stored category ids.
// Canonical: "all", "opc_read", "db_write", "opc_write".
func NormalizeCategory(category string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "", "all":
		return "all"
	case CategoryOPCRead, "opc", "opc-read", "opcread":
		return CategoryOPCRead
	case CategoryDBWrite, "db", "db-write", "dbwrite":
		return CategoryDBWrite
	case CategoryOPCWrite, "opc-write", "opcwrite", "write":
		return CategoryOPCWrite
	default:
		return category
	}
}

// Query returns newest entries first, filtered.
func (b *Buffer) Query(category string, errorsOnly bool, limit int) []Entry {
	if limit <= 0 || limit > b.max {
		limit = 500
	}
	category = NormalizeCategory(category)
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]Entry, 0, limit)
	for i := len(b.entries) - 1; i >= 0 && len(out) < limit; i-- {
		e := b.entries[i]
		if category != "all" && e.Category != category {
			continue
		}
		if errorsOnly && !isErrorLevel(e.Level) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func isErrorLevel(level string) bool {
	switch strings.ToLower(level) {
	case LevelWarn, LevelError:
		return true
	default:
		return false
	}
}

func record(e Entry) {
	if defaultBuf == nil {
		return
	}
	defaultBuf.Add(e)
}

// OPCRead logs OPC UA read / poll events.
func OPCRead(level, deviceID, tagID, message, detail string) {
	record(Entry{
		Level: level, Category: CategoryOPCRead,
		DeviceID: deviceID, TagID: tagID,
		Message: message, Detail: detail,
	})
}

// DBWrite logs historian batch writes and spool activity.
func DBWrite(level, message, detail string, count int) {
	record(Entry{
		Level: level, Category: CategoryDBWrite,
		Message: message, Detail: detail, Count: count,
	})
}

// OPCWrite logs OPC UA write attempts (REST PUT → PLC).
func OPCWrite(level, deviceID, tagID, message, detail string) {
	record(Entry{
		Level: level, Category: CategoryOPCWrite,
		DeviceID: deviceID, TagID: tagID,
		Message: message, Detail: detail,
	})
}
