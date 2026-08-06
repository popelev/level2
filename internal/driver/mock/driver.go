package mock

import (
	"context"
	"fmt"
	"math"
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
	auto      bool // synthesize values from tag datatype
	started   time.Time
}

func New() *Driver {
	return &Driver{
		values:   make(map[string]core.Sample),
		interval: 50 * time.Millisecond,
	}
}

// NewDemo publishes synthetic Good samples for PLC-off UI/API/history.
func NewDemo(interval time.Duration) *Driver {
	if interval <= 0 {
		interval = time.Second
	}
	return &Driver{
		values:   make(map[string]core.Sample),
		interval: interval,
		auto:     true,
		started:  time.Now(),
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
			now := time.Now().UTC()
			d.mu.Lock()
			for _, t := range tags {
				if !t.Enabled {
					continue
				}
				var s core.Sample
				if d.auto {
					s = synthesize(t, now, time.Since(d.started))
				} else if v, ok := d.values[t.ID]; ok {
					s = v
					s.Time = now
				} else {
					s = core.Sample{TagID: t.ID, Quality: core.QualityBad, Time: now}
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

func synthesize(t core.Tag, now time.Time, age time.Duration) core.Sample {
	s := core.Sample{TagID: t.ID, Time: now, Quality: core.QualityGood}
	sec := age.Seconds()
	switch t.DataType {
	case core.ValueFloat64:
		n := 90 + 5*math.Sin(sec/5)
		s.ValueNum = &n
	case core.ValueInt64:
		n := float64(int64(100 + 10*math.Sin(sec/7)))
		s.ValueNum = &n
	case core.ValueUint:
		n := float64(uint64(50 + 20*math.Sin(sec/6)))
		s.ValueNum = &n
	case core.ValueBool:
		b := int(sec/3)%2 == 0
		s.ValueBool = &b
	case core.ValueString:
		u := "demo"
		s.ValueText = &u
	case core.ValueDateTime:
		u := time.Now().UTC().Format(time.RFC3339Nano)
		s.ValueText = &u
	default:
		s.Quality = core.QualityBad
	}
	return s
}
