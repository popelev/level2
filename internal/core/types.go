package core

import (
	"context"
	"time"
)

// ValueType is the platform scalar type stored in the historian.
type ValueType string

const (
	ValueBool     ValueType = "bool"
	ValueInt64    ValueType = "int64"
	ValueUint     ValueType = "uint"
	ValueFloat64  ValueType = "float64"
	ValueString   ValueType = "string"
	ValueDateTime ValueType = "datetime"
)

// Quality mirrors OPC UA status at a coarse level.
type Quality int

const (
	QualityGood Quality = 0
	QualityBad  Quality = 1
)

// Sample is one historian point.
type Sample struct {
	Time      time.Time `json:"time"`
	TagID     string    `json:"tag_id"`
	ValueNum  *float64  `json:"value_num,omitempty"`
	ValueText *string   `json:"value_text,omitempty"`
	ValueBool *bool     `json:"value_bool,omitempty"`
	Quality   Quality   `json:"quality"`
}

// SamePayload reports whether value fields and quality match.
// Time and TagID are ignored (used for historian on-change suppress).
func (s Sample) SamePayload(other Sample) bool {
	if s.Quality != other.Quality {
		return false
	}
	if !ptrFloat64Equal(s.ValueNum, other.ValueNum) {
		return false
	}
	if !ptrStringEqual(s.ValueText, other.ValueText) {
		return false
	}
	return ptrBoolEqual(s.ValueBool, other.ValueBool)
}

func ptrFloat64Equal(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func ptrStringEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func ptrBoolEqual(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// PollMode is how the collector obtains tag values from the field.
// Empty Mode on Tag means PollModePoll (legacy).
type PollMode string

const (
	PollModePoll      PollMode = "poll"      // periodic Read
	PollModeSubscribe PollMode = "subscribe" // OPC UA MonitoredItems / on-change (Phase 2)
)

// Tag describes a monitored leaf node.
type Tag struct {
	ID         string    `yaml:"id" json:"id"`
	NodeID     string    `yaml:"node_id" json:"node_id"` // e.g. ns=4;i=4208
	Path       string    `yaml:"path,omitempty" json:"path,omitempty"` // UI tree folders, e.g. Area/Path
	DataType   ValueType `yaml:"datatype" json:"datatype"`
	Enabled    bool      `yaml:"enabled" json:"enabled"`
	IntervalMs int       `yaml:"interval_ms" json:"interval_ms"`
	// Writable allows Level2 Write API / UI to set this tag (default false).
	// Independent of Siemens AccessLevel — server can still deny.
	Writable bool `yaml:"writable,omitempty" json:"writable"`
	// Simulate feeds synthetic Good samples for this tag (default false).
	// Real OPC collect continues for tags with simulate=false. Never auto-enabled on disconnect.
	// See docs/tag-simulation.md.
	Simulate bool `yaml:"simulate,omitempty" json:"simulate"`
	// Mode is poll (legacy) or subscribe (OPC Subscription). See docs/opc-subscription-mode.md.
	Mode PollMode `yaml:"mode,omitempty" json:"mode,omitempty"`
}

// Parallel OPC Read workers per device poll (batches of ≤100 nodes).
const (
	DefaultPollConcurrency = 4
	MinPollConcurrency     = 1
	MaxPollConcurrency     = 16
)

// NormalizePollConcurrency applies default (4) when n≤0 and clamps to 1–16.
func NormalizePollConcurrency(n int) int {
	if n <= 0 {
		return DefaultPollConcurrency
	}
	if n < MinPollConcurrency {
		return MinPollConcurrency
	}
	if n > MaxPollConcurrency {
		return MaxPollConcurrency
	}
	return n
}

// Device is an OPC UA endpoint.
type Device struct {
	ID       string `yaml:"id" json:"id"`
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"` // may come from env
	Security string `yaml:"security" json:"security"` // "None" for lab
	// PollConcurrency is how many OPC Read batches may run in parallel for this device (1–16; default 4).
	PollConcurrency int    `yaml:"poll_concurrency,omitempty" json:"poll_concurrency,omitempty"`
	Tags            []Tag  `yaml:"tags" json:"tags"`
}

// Driver reads live values from a field protocol.
type Driver interface {
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	// Subscribe delivers samples; for M1 may be implemented via polling.
	Subscribe(ctx context.Context, tags []Tag, out chan<- Sample) error
	Connected() bool
}

// ValueWriter writes a scalar value to a configured tag's OPC node (optional driver capability).
// value is a Go scalar already coerced to the tag datatype (bool, int64, uint32/uint64, float64, string, time.Time).
type ValueWriter interface {
	WriteValue(ctx context.Context, tag Tag, value any) error
}

// ValueReader reads a scalar value from a configured tag's OPC node (optional driver capability).
// Used for write-then-verify readback; returns a Sample mapped like the poll path.
type ValueReader interface {
	ReadValue(ctx context.Context, tag Tag) (Sample, error)
}

// DataTypeResolver resolves tag datatypes from the field protocol (optional driver capability).
// Used by the API for empty-datatype fill and Sync-from-OPC without coupling to a concrete driver.
type DataTypeResolver interface {
	ResolveTagDataType(ctx context.Context, nodeID, browseHint string) ValueType
	// ApplyDataTypes overwrites each tag's DataType from protocol metadata (batched when possible).
	ApplyDataTypes(ctx context.Context, tags []Tag)
}

// Historian stores time series.
type Historian interface {
	EnsureSchema(ctx context.Context) error
	WriteBatch(ctx context.Context, samples []Sample) error
	Close(ctx context.Context) error
}

// HistoryQuerier is optional historian capability for the API.
type HistoryQuerier interface {
	QueryHistory(ctx context.Context, tagID string, from, to time.Time, limit int) ([]Sample, error)
}

// Browser expands / browses OPC address space (optional driver capability).
type Browser interface {
	BrowseChildren(ctx context.Context, parentNodeID string) ([]BrowseNode, error)
	ExpandStructure(ctx context.Context, parentNodeID, parentTagID string, maxDepth int) ([]ExpandedTag, error)
}

// BrowseNode is duplicated lightly in core for API decoupling from driver package.
type BrowseNode struct {
	NodeID      string `json:"node_id"`
	BrowseName  string `json:"browse_name"`
	DisplayName string `json:"display_name"`
	NodeClass   string `json:"node_class"`
	IsLeaf      bool   `json:"is_leaf"`
	DataType    string `json:"datatype,omitempty"`
}

type ExpandedTag struct {
	ID         string    `json:"id"`
	NodeID     string    `json:"node_id"`
	BrowsePath string    `json:"browse_path"`
	DataType   ValueType `json:"datatype"`
}
