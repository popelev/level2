package spool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/popelev/level2/internal/core"
)

// FileSpool persists failed historian batches as JSON lines for later replay.
type FileSpool struct {
	mu   sync.Mutex
	dir  string
	maxN int
}

func New(dir string, maxFiles int) (*FileSpool, error) {
	if maxFiles <= 0 {
		maxFiles = 1000
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &FileSpool{dir: dir, maxN: maxFiles}, nil
}

func (s *FileSpool) Enqueue(samples []core.Sample) error {
	if len(samples) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	files, _ := filepath.Glob(filepath.Join(s.dir, "*.json"))
	if len(files) >= s.maxN {
		return fmt.Errorf("spool full (%d files)", len(files))
	}
	name := filepath.Join(s.dir, fmt.Sprintf("%d.json", time.Now().UTC().UnixNano()))
	b, err := json.Marshal(samples)
	if err != nil {
		return err
	}
	return os.WriteFile(name, b, 0o644)
}

// Drain loads up to limit batches (oldest first) without deleting.
func (s *FileSpool) List() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := filepath.Glob(filepath.Join(s.dir, "*.json"))
	if err != nil {
		return nil, err
	}
	return files, nil
}

func (s *FileSpool) Load(path string) ([]core.Sample, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var samples []core.Sample
	if err := json.Unmarshal(b, &samples); err != nil {
		return nil, err
	}
	return samples, nil
}

func (s *FileSpool) Remove(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.Remove(path)
}

func (s *FileSpool) Len() int {
	files, _ := s.List()
	return len(files)
}
