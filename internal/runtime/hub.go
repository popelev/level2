package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/popelev/level2/internal/core"
	opcuaDriver "github.com/popelev/level2/internal/driver/opcua"
	"github.com/popelev/level2/internal/driver/simbrowser"
)

// Hub tracks per-device OPC drivers / browsers (multi-server like UaExpert).
type Hub struct {
	mu      sync.RWMutex
	log     *slog.Logger
	useSim  bool
	entries map[string]*Entry
}

type Entry struct {
	Device    core.Device
	Driver    core.Driver
	Browser   core.Browser
	Connected bool
}

func NewHub(log *slog.Logger, useSim bool) *Hub {
	return &Hub{
		log:     log,
		useSim:  useSim,
		entries: make(map[string]*Entry),
	}
}

func (h *Hub) Upsert(ctx context.Context, dev core.Device) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if old, ok := h.entries[dev.ID]; ok {
		_ = old.Driver.Disconnect(ctx)
		delete(h.entries, dev.ID)
	}
	ent, err := h.newEntry(ctx, dev)
	if err != nil {
		return err
	}
	h.entries[dev.ID] = ent
	return nil
}

func (h *Hub) Remove(ctx context.Context, id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if old, ok := h.entries[id]; ok {
		_ = old.Driver.Disconnect(ctx)
		delete(h.entries, id)
	}
}

func (h *Hub) Browser(deviceID string) (core.Browser, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ent, ok := h.entries[deviceID]
	if !ok || ent.Browser == nil {
		return nil, fmt.Errorf("no browser for device %q", deviceID)
	}
	return ent.Browser, nil
}

func (h *Hub) Status() map[string]bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[string]bool, len(h.entries))
	for id, e := range h.entries {
		out[id] = e.Connected && e.Driver.Connected()
	}
	return out
}

func (h *Hub) AnyConnected() bool {
	for _, ok := range h.Status() {
		if ok {
			return true
		}
	}
	return false
}

func (h *Hub) Drivers() []core.Driver {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]core.Driver, 0, len(h.entries))
	for _, e := range h.entries {
		out = append(out, e.Driver)
	}
	return out
}

func (h *Hub) Entry(id string) (*Entry, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	e, ok := h.entries[id]
	return e, ok
}

func (h *Hub) newEntry(ctx context.Context, dev core.Device) (*Entry, error) {
	if h.useSim {
		demo := simbrowser.NewDemo()
		// lightweight connected stub for readyz / UI
		stub := &alwaysOn{}
		return &Entry{Device: dev, Driver: stub, Browser: demo, Connected: true}, nil
	}
	d := opcuaDriver.New(dev, h.log.With("device", dev.ID))
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	err := d.Connect(cctx)
	cancel()
	ent := &Entry{Device: dev, Driver: d, Browser: d, Connected: err == nil}
	if err != nil {
		h.log.Warn("opc connect", "device", dev.ID, "err", err)
	}
	return ent, nil
}

type alwaysOn struct{}

func (a *alwaysOn) Connect(context.Context) error    { return nil }
func (a *alwaysOn) Disconnect(context.Context) error { return nil }
func (a *alwaysOn) Connected() bool                  { return true }
func (a *alwaysOn) Subscribe(context.Context, []core.Tag, chan<- core.Sample) error {
	return fmt.Errorf("sim stub has no subscribe")
}
