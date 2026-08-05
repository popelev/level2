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

func (s *Store) saveLocked() error {
	out := *s.file
	out.Devices = append([]core.Device(nil), s.file.Devices...)
	for i := range out.Devices {
		out.Devices[i].Tags = append([]core.Tag(nil), s.file.Devices[i].Tags...)
		// Keep secrets in env (.env); do not rewrite passwords into YAML on import.
		out.Devices[i].Password = ""
	}
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
