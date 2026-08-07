package config

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/popelev/level2/internal/core"
	"gopkg.in/yaml.v3"
)

// Store is a mutable config used by API imports and collection loops.
type Store struct {
	mu   sync.RWMutex
	path string
	file *File
	gen  atomic.Uint64
}

func NewStore(path string, file *File) *Store {
	s := &Store{path: path, file: file}
	s.gen.Store(1)
	return s
}

func (s *Store) Gen() uint64 { return s.gen.Load() }

func (s *Store) Snapshot() File {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := *s.file
	cp.Devices = append([]core.Device(nil), s.file.Devices...)
	for i := range cp.Devices {
		cp.Devices[i].Tags = append([]core.Tag(nil), s.file.Devices[i].Tags...)
	}
	return cp
}

func (s *Store) Devices() []core.Device {
	snap := s.Snapshot()
	return snap.Devices
}

// OPCWriteEnabled reports whether PLC write API is allowed (opc_write_enabled / LEVEL2_OPC_WRITE_ENABLED).
func (s *Store) OPCWriteEnabled() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.file == nil {
		return false
	}
	return s.file.OPCWriteEnabled
}

// TagSimulation reports whether synthetic tag samples are enabled (tag_simulation / LEVEL2_TAG_SIMULATION).
// Default false; never inferred from OPC disconnect.
func (s *Store) TagSimulation() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.file == nil {
		return false
	}
	return s.file.TagSimulation
}

// SetTagSimulation persists the opt-in flag. Collector applies sample simulation on process start
// (compose recreate); PUT API returns restart_required until then.
func (s *Store) SetTagSimulation(enabled bool) error {
	if s == nil {
		return fmt.Errorf("nil config store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return fmt.Errorf("nil config file")
	}
	s.file.TagSimulation = enabled
	return s.saveLocked()
}

// APIToken returns the shared API token (empty = auth disabled).
func (s *Store) APIToken() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.file == nil {
		return ""
	}
	return s.file.APIToken
}

func (s *Store) AllTags() []core.Tag {
	var tags []core.Tag
	for _, d := range s.Devices() {
		tags = append(tags, d.Tags...)
	}
	return tags
}

func (s *Store) DeviceTags(deviceID string) ([]core.Tag, error) {
	for _, d := range s.Devices() {
		if d.ID == deviceID {
			return append([]core.Tag(nil), d.Tags...), nil
		}
	}
	return nil, fmt.Errorf("device %q not found", deviceID)
}

// MergeTags upserts tags on a device by tag id and persists YAML.
// replace=true removes existing tags first.
func (s *Store) MergeTags(deviceID string, tags []core.Tag, replace bool) (added, updated int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i := range s.file.Devices {
		if s.file.Devices[i].ID == deviceID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, 0, fmt.Errorf("device %q not found", deviceID)
	}
	dev := &s.file.Devices[idx]
	if replace {
		dev.Tags = nil
	}
	byID := map[string]int{}
	for i, t := range dev.Tags {
		byID[t.ID] = i
	}
	for _, t := range tags {
		if t.IntervalMs <= 0 {
			t.IntervalMs = 1000
		}
		if j, ok := byID[t.ID]; ok {
			dev.Tags[j] = t
			updated++
		} else {
			dev.Tags = append(dev.Tags, t)
			byID[t.ID] = len(dev.Tags) - 1
			added++
		}
	}
	if err := s.saveLocked(); err != nil {
		return 0, 0, err
	}
	s.gen.Add(1)
	return added, updated, nil
}

func (s *Store) deviceIndexLocked(deviceID string) (int, error) {
	for i := range s.file.Devices {
		if s.file.Devices[i].ID == deviceID {
			return i, nil
		}
	}
	return -1, fmt.Errorf("device %q not found", deviceID)
}

// UpsertTag creates or updates one monitored tag (written to DB when enabled).
func (s *Store) UpsertTag(deviceID string, tag core.Tag) error {
	if tag.ID == "" || tag.NodeID == "" {
		return fmt.Errorf("tag id and node_id required")
	}
	if _, err := core.ParseNodeID(tag.NodeID); err != nil {
		return err
	}
	if tag.IntervalMs <= 0 {
		tag.IntervalMs = 1000
	}
	tag.DataType = core.NormalizeValueType(tag.DataType)
	if tag.DataType == "" {
		tag.DataType = core.ValueFloat64
	}
	if !core.ValidValueType(tag.DataType) {
		return fmt.Errorf("unsupported datatype %q", tag.DataType)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.deviceIndexLocked(deviceID)
	if err != nil {
		return err
	}
	dev := &s.file.Devices[idx]
	for i := range dev.Tags {
		if dev.Tags[i].ID == tag.ID {
			dev.Tags[i] = tag
			if err := s.saveLocked(); err != nil {
				return err
			}
			s.gen.Add(1)
			return nil
		}
	}
	dev.Tags = append(dev.Tags, tag)
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.gen.Add(1)
	return nil
}

func (s *Store) DeleteTag(deviceID, tagID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.deviceIndexLocked(deviceID)
	if err != nil {
		return err
	}
	dev := &s.file.Devices[idx]
	out := dev.Tags[:0]
	found := false
	for _, t := range dev.Tags {
		if t.ID == tagID {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		return fmt.Errorf("tag %q not found", tagID)
	}
	dev.Tags = out
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.gen.Add(1)
	return nil
}

// SetDeviceTags replaces the full tag list for a device (single persist).
func (s *Store) SetDeviceTags(deviceID string, tags []core.Tag) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.deviceIndexLocked(deviceID)
	if err != nil {
		return err
	}
	cp := make([]core.Tag, len(tags))
	copy(cp, tags)
	s.file.Devices[idx].Tags = cp
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.gen.Add(1)
	return nil
}

// ClearDeviceTags removes all tags from a device.
func (s *Store) ClearDeviceTags(deviceID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.deviceIndexLocked(deviceID)
	if err != nil {
		return 0, err
	}
	n := len(s.file.Devices[idx].Tags)
	s.file.Devices[idx].Tags = nil
	if err := s.saveLocked(); err != nil {
		return 0, err
	}
	s.gen.Add(1)
	return n, nil
}

// UpsertDevice creates or updates a device (keeps existing tags on update unless cleared).
func (s *Store) UpsertDevice(dev core.Device) error {
	if dev.ID == "" {
		return fmt.Errorf("device id required")
	}
	if dev.Endpoint == "" {
		return fmt.Errorf("endpoint required")
	}
	if dev.Security == "" {
		dev.Security = "None"
	}
	dev.PollConcurrency = core.NormalizePollConcurrency(dev.PollConcurrency)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.file.Devices {
		if s.file.Devices[i].ID == dev.ID {
			prev := s.file.Devices[i]
			if len(dev.Tags) == 0 {
				dev.Tags = prev.Tags
			}
			if dev.Password == "" {
				dev.Password = prev.Password
			}
			s.file.Devices[i] = dev
			if err := s.saveLocked(); err != nil {
				return err
			}
			s.gen.Add(1)
			return nil
		}
	}
	if dev.Tags == nil {
		dev.Tags = []core.Tag{}
	}
	s.file.Devices = append(s.file.Devices, dev)
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.gen.Add(1)
	return nil
}

func (s *Store) DeleteDevice(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.file.Devices[:0]
	found := false
	for _, d := range s.file.Devices {
		if d.ID == id {
			found = true
			continue
		}
		out = append(out, d)
	}
	if !found {
		return fmt.Errorf("device %q not found", id)
	}
	s.file.Devices = out
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.gen.Add(1)
	return nil
}

// ApplyProject merges or replaces devices from a project snapshot.
// Passwords are never taken from the project file; existing passwords are kept on match.
func (s *Store) ApplyProject(devices []core.Device, replace bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prevPass := map[string]string{}
	for _, d := range s.file.Devices {
		prevPass[d.ID] = d.Password
	}
	if replace {
		s.file.Devices = nil
	}
	byID := map[string]int{}
	for i, d := range s.file.Devices {
		byID[d.ID] = i
	}
	for _, d := range devices {
		if d.ID == "" || d.Endpoint == "" {
			continue
		}
		if d.Security == "" {
			d.Security = "None"
		}
		d.PollConcurrency = core.NormalizePollConcurrency(d.PollConcurrency)
		d.Password = prevPass[d.ID]
		if d.Tags == nil {
			d.Tags = []core.Tag{}
		}
		if i, ok := byID[d.ID]; ok {
			if !replace && len(d.Tags) == 0 {
				d.Tags = s.file.Devices[i].Tags
			}
			s.file.Devices[i] = d
		} else {
			s.file.Devices = append(s.file.Devices, d)
			byID[d.ID] = len(s.file.Devices) - 1
		}
	}
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.gen.Add(1)
	return nil
}

// SetCapacityPolicy updates disk capacity percent and full-disk policy, then persists YAML.
func (s *Store) SetCapacityPolicy(percent int, policy string) error {
	percent, policy, err := ValidateCapacityPolicy(percent, policy)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.file.Database.CapacityPercent = percent
	s.file.Database.FullPolicy = policy
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.gen.Add(1)
	return nil
}

func (s *Store) saveLocked() error {
	out := *s.file
	out.Devices = append([]core.Device(nil), s.file.Devices...)
	for i := range out.Devices {
		out.Devices[i].Tags = append([]core.Tag(nil), s.file.Devices[i].Tags...)
		// Keep secrets in env (.env); do not rewrite passwords into YAML on import.
		out.Devices[i].Password = ""
	}
	// Prefer LEVEL2_API_TOKEN env; do not persist token into rewritten YAML.
	out.APIToken = ""
	b, err := yaml.Marshal(&out)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
