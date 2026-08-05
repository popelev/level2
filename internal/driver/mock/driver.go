package mock

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/popelev/level2/internal/core"
)

// Driver is an in-memory driver for PLC-off tests and demos.
type Driver struct {
	mu        sync.Mutex
	alive     atomic.Bool
	failUntil time.Time
	values    map[string]core.Sample
	interval  time.Duration
	ConnectFn func(ctx context.Context) error
}

func New() *Driver {
	return &Driver{
		values:   make(map[string]core.Sample),
		interval: 50 * time.Millisecond,
	}
}

func (d *Driver) SetValue(tagID string, s core.Sample) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s.TagID = tagID
	d.values[tagID] = s
}

func (d *Driver) FailConnectFor(durr time.Duration) {
	d.failUntil = time.Now().Add(durr)
}

func (d *Driver) Connected() bool { return d.alive.Load() }

func (d *Driver) Connect(ctx context.Context) error {
	if d.ConnectFn != nil {
		if err := d.ConnectFn(ctx); err != nil {
			d.alive.Store(false)
			return err
		}
	}
	if time.Now().Before(d.failUntil) {
		d.alive.Store(false)
		return fmt.Errorf("mock connect refused")
	}
	d.alive.Store(true)
	return nil
}

func (d *Driver) Disconnect(ctx context.Context) error {
	d.alive.Store(false)
	return nil
}

func (d *Driver) Subscribe(ctx context.Context, tags []core.Tag, out chan<- core.Sample) error {
	if !d.Connected() {
		return fmt.Errorf("not connected")
	}
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			d.mu.Lock()
			for _, t := range tags {
				if !t.Enabled {
					continue
				}
				s, ok := d.values[t.ID]
				if !ok {
					s = core.Sample{TagID: t.ID, Quality: core.QualityBad, Time: time.Now().UTC()}
				} else {
					s.Time = time.Now().UTC()
				}
				select {
				case out <- s:
				case <-ctx.Done():
					d.mu.Unlock()
					return ctx.Err()
				}
			}
			d.mu.Unlock()
		}
	}
}
