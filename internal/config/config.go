package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/popelev/level2/internal/core"
	"gopkg.in/yaml.v3"
)

// File is the YAML configuration root.
type File struct {
	Listen   string        `yaml:"listen"`
	Database Database      `yaml:"database"`
	SpoolDir string        `yaml:"spool_dir"`
	UIDir    string        `yaml:"ui_dir"`
	// OPCWriteEnabled gates PUT /api/v1/tags/{id}/value. Default false; override with LEVEL2_OPC_WRITE_ENABLED.
	OPCWriteEnabled bool `yaml:"opc_write_enabled"`
	// APIToken, when non-empty, requires Bearer / X-API-Token on mutating /api/v1 routes (and WS).
	// Empty = auth disabled (lab backward compatible). Override with LEVEL2_API_TOKEN.
	APIToken string        `yaml:"api_token,omitempty"`
	Devices  []core.Device `yaml:"devices"`
}

// Full-disk policies when used bytes reach the capacity percent limit.
const (
	FullPolicyStop        = "stop"
	FullPolicyDropOldest  = "drop_oldest"
	FullPolicyRotate      = "rotate"       // Phase 2 stub — behaves like stop
	FullPolicyExpandLimit = "expand_limit" // user raises percent; until then like stop
)

type Database struct {
	URL              string `yaml:"url"`
	CapacityPercent  int    `yaml:"capacity_percent,omitempty"` // 1–100; max fraction of disk DB may use
	FullPolicy       string `yaml:"full_policy,omitempty"`      // stop | drop_oldest | rotate | expand_limit
}

// Load reads YAML and applies env overrides for secrets/endpoint.
func Load(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if f.Listen == "" {
		f.Listen = ":8080"
	}
	if f.SpoolDir == "" {
		f.SpoolDir = "/var/lib/level2/spool"
	}
	if f.UIDir == "" {
		f.UIDir = "/usr/share/level2/ui"
	}
	if u := os.Getenv("DATABASE_URL"); u != "" {
		f.Database.URL = u
	}
	if f.Database.URL == "" {
		return nil, fmt.Errorf("database.url is required (or DATABASE_URL env)")
	}
	if err := normalizeDatabaseCapacity(&f.Database); err != nil {
		return nil, err
	}
	if v := strings.TrimSpace(os.Getenv("LEVEL2_DB_CAPACITY_PERCENT")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("LEVEL2_DB_CAPACITY_PERCENT: %w", err)
		}
		f.Database.CapacityPercent = n
		if err := normalizeDatabaseCapacity(&f.Database); err != nil {
			return nil, err
		}
	}
	if v := strings.TrimSpace(os.Getenv("LEVEL2_DB_FULL_POLICY")); v != "" {
		f.Database.FullPolicy = v
		if err := normalizeDatabaseCapacity(&f.Database); err != nil {
			return nil, err
		}
	}
	if v := os.Getenv("SPOOL_DIR"); v != "" {
		f.SpoolDir = v
	}
	if v := os.Getenv("UI_DIR"); v != "" {
		f.UIDir = v
	}
	if v := strings.TrimSpace(os.Getenv("LEVEL2_OPC_WRITE_ENABLED")); v != "" {
		enabled, err := parseEnvBool(v)
		if err != nil {
			return nil, fmt.Errorf("LEVEL2_OPC_WRITE_ENABLED: %w", err)
		}
		f.OPCWriteEnabled = enabled
	}
	if v, ok := os.LookupEnv("LEVEL2_API_TOKEN"); ok {
		f.APIToken = strings.TrimSpace(v)
	}
	for i := range f.Devices {
		d := &f.Devices[i]
		ApplyDeviceEnvOverlay(d)
		if d.Security == "" {
			d.Security = "None"
		}
		d.PollConcurrency = core.NormalizePollConcurrency(d.PollConcurrency)
		if d.Endpoint == "" {
			return nil, fmt.Errorf("device %q: endpoint required", d.ID)
		}
		for j := range d.Tags {
			t := &d.Tags[j]
			if t.ID == "" {
				return nil, fmt.Errorf("device %q: tag missing id", d.ID)
			}
			if t.NodeID == "" {
				return nil, fmt.Errorf("tag %q: node_id required", t.ID)
			}
			if _, err := core.ParseNodeID(t.NodeID); err != nil {
				return nil, fmt.Errorf("tag %q: %w", t.ID, err)
			}
			t.DataType = core.NormalizeValueType(core.ValueType(strings.ToLower(string(t.DataType))))
			if t.DataType == "" {
				t.DataType = core.ValueFloat64
			}
			if !core.ValidValueType(t.DataType) {
				return nil, fmt.Errorf("tag %q: unsupported datatype %q", t.ID, t.DataType)
			}
			if t.IntervalMs <= 0 {
				t.IntervalMs = 1000
			}
		}
	}
	return &f, nil
}

func normalizeDatabaseCapacity(db *Database) error {
	if db.CapacityPercent == 0 {
		db.CapacityPercent = 90
	}
	if db.CapacityPercent < 1 || db.CapacityPercent > 100 {
		return fmt.Errorf("database.capacity_percent must be 1–100, got %d", db.CapacityPercent)
	}
	if db.FullPolicy == "" {
		db.FullPolicy = FullPolicyStop
	}
	db.FullPolicy = strings.ToLower(strings.TrimSpace(db.FullPolicy))
	if !ValidFullPolicy(db.FullPolicy) {
		return fmt.Errorf("database.full_policy must be one of stop|drop_oldest|rotate|expand_limit, got %q", db.FullPolicy)
	}
	return nil
}

func parseEnvBool(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("want true/false/1/0, got %q", v)
	}
}

// ValidFullPolicy reports whether p is a known full-disk policy.
func ValidFullPolicy(p string) bool {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case FullPolicyStop, FullPolicyDropOldest, FullPolicyRotate, FullPolicyExpandLimit:
		return true
	default:
		return false
	}
}

// ValidateCapacityPolicy checks percent/policy without applying defaults (for API updates).
func ValidateCapacityPolicy(percent int, policy string) (int, string, error) {
	policy = strings.ToLower(strings.TrimSpace(policy))
	if percent < 1 || percent > 100 {
		return 0, "", fmt.Errorf("capacity_percent must be 1–100, got %d", percent)
	}
	if policy == "" {
		policy = FullPolicyStop
	}
	if !ValidFullPolicy(policy) {
		return 0, "", fmt.Errorf("full_policy must be one of stop|drop_oldest|rotate|expand_limit, got %q", policy)
	}
	return percent, policy, nil
}
