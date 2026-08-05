package core

import (
	"context"
	"time"
)

// ValueType is the platform scalar type stored in the historian.
type ValueType string

const (
	ValueBool    ValueType = "bool"
	ValueInt64   ValueType = "int64"
	ValueFloat64 ValueType = "float64"
	ValueString  ValueType = "string"
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

// Tag describes a monitored leaf node.
type Tag struct {
	ID         string    `yaml:"id" json:"id"`
	NodeID     string    `yaml:"node_id" json:"node_id"` // e.g. ns=4;i=4208
	Path       string    `yaml:"path,omitempty" json:"path,omitempty"` // UI tree folders, e.g. Area/Path
	DataType   ValueType `yaml:"datatype" json:"datatype"`
	Enabled    bool      `yaml:"enabled" json:"enabled"`
	IntervalMs int       `yaml:"interval_ms" json:"interval_ms"`
}

// Device is an OPC UA endpoint.
type Device struct {
	ID       string `yaml:"id" json:"id"`
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"` // may come from env
	Security string `yaml:"security" json:"security"` // "None" for lab
	Tags     []Tag  `yaml:"tags" json:"tags"`
}

// Driver reads live values from a field protocol.
type Driver interface {
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	// Subscribe delivers samples; for M1 may be implemented via polling.
	Subscribe(ctx context.Context, tags []Tag, out chan<- Sample) error
	Connected() bool
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
}

type ExpandedTag struct {
	ID         string    `json:"id"`
	NodeID     string    `json:"node_id"`
	BrowsePath string    `json:"browse_path"`
	DataType   ValueType `json:"datatype"`
}
