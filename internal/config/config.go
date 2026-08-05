package config

import (
	"fmt"
	"os"
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
	Devices  []core.Device `yaml:"devices"`
}

type Database struct {
	URL string `yaml:"url"`
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
	if v := os.Getenv("SPOOL_DIR"); v != "" {
		f.SpoolDir = v
	}
	if v := os.Getenv("UI_DIR"); v != "" {
		f.UIDir = v
	}
	for i := range f.Devices {
		d := &f.Devices[i]
		if ep := os.Getenv("PLC_OPC_ENDPOINT"); ep != "" && i == 0 {
			d.Endpoint = ep
		}
		if u := os.Getenv("OPC_UA_USERNAME"); u != "" {
			d.Username = u
		}
		if p := os.Getenv("OPC_UA_PASSWORD"); p != "" {
			d.Password = p
		}
		if d.Security == "" {
			d.Security = "None"
		}
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
			t.DataType = core.ValueType(strings.ToLower(string(t.DataType)))
			switch t.DataType {
			case core.ValueBool, core.ValueInt64, core.ValueFloat64, core.ValueString:
			case "":
				t.DataType = core.ValueFloat64
			default:
				return nil, fmt.Errorf("tag %q: unsupported datatype %q", t.ID, t.DataType)
			}
			if t.IntervalMs <= 0 {
				t.IntervalMs = 1000
			}
		}
	}
	return &f, nil
}
