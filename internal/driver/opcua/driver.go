package opcua

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
)

// Driver is an OPC UA client that polls leaf nodes (M1).
// Structure nodes are rejected at value-map time — expand comes in M2.
type Driver struct {
	device core.Device
	log    *slog.Logger

	mu     sync.Mutex
	client *opcua.Client
	alive  atomic.Bool
}

func New(device core.Device, log *slog.Logger) *Driver {
	if log == nil {
		log = slog.Default()
	}
	return &Driver{device: device, log: log}
}

func (d *Driver) Connected() bool { return d.alive.Load() }

func (d *Driver) Connect(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.client != nil {
		return nil
	}

	opts := []opcua.Option{
		opcua.SecurityMode(ua.MessageSecurityModeNone),
		opcua.SecurityPolicy(ua.SecurityPolicyURINone),
	}
	if d.device.Username != "" {
		opts = append(opts, opcua.AuthUsername(d.device.Username, d.device.Password))
	} else {
		opts = append(opts, opcua.AuthAnonymous())
	}

	c, err := opcua.NewClient(d.device.Endpoint, opts...)
	if err != nil {
		return fmt.Errorf("opcua new client: %w", err)
	}
	if err := c.Connect(ctx); err != nil {
		return fmt.Errorf("opcua connect %s: %w", d.device.Endpoint, err)
	}
	d.client = c
	d.alive.Store(true)
	d.log.Info("opcua connected", "endpoint", d.device.Endpoint, "device", d.device.ID)
	return nil
}

func (d *Driver) Disconnect(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.alive.Store(false)
	if d.client == nil {
		return nil
	}
	err := d.client.Close(ctx)
	d.client = nil
	return err
}

// Subscribe implements core.Driver by polling enabled leaf tags.
func (d *Driver) Subscribe(ctx context.Context, tags []core.Tag, out chan<- core.Sample) error {
	views, err := PrepareTags(tags)
	if err != nil {
		return err
	}
	enabled := make([]TagView, 0, len(views))
	for _, t := range views {
		if t.Enabled {
			enabled = append(enabled, t)
		}
	}
	if len(enabled) == 0 {
		return fmt.Errorf("no enabled tags")
	}

	interval := time.Duration(enabled[0].IntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := d.pollOnce(ctx, enabled, out); err != nil {
				d.log.Warn("opcua poll failed", "err", err)
				d.alive.Store(false)
				_ = d.Disconnect(ctx)
				if cerr := d.Connect(ctx); cerr != nil {
					d.log.Error("opcua reconnect failed", "err", cerr)
				}
			}
		}
	}
}

// TagView is driver-local tag with parsed node.
type TagView struct {
	core.Tag
	Parsed core.ParsedNodeID
}

func ValidateLeafTag(t TagView) error {
	switch t.DataType {
	case core.ValueBool, core.ValueInt64, core.ValueFloat64, core.ValueString:
		return nil
	default:
		return fmt.Errorf("tag %q: unsupported datatype %q", t.ID, t.DataType)
	}
}

func PrepareTags(tags []core.Tag) ([]TagView, error) {
	out := make([]TagView, 0, len(tags))
	for _, t := range tags {
		p, err := core.ParseNodeID(t.NodeID)
		if err != nil {
			return nil, fmt.Errorf("tag %q: %w", t.ID, err)
		}
		tv := TagView{Tag: t, Parsed: p}
		if err := ValidateLeafTag(tv); err != nil {
			return nil, err
		}
		out = append(out, tv)
	}
	return out, nil
}

func (d *Driver) pollOnce(ctx context.Context, tags []TagView, out chan<- core.Sample) error {
	d.mu.Lock()
	c := d.client
	d.mu.Unlock()
	if c == nil {
		return fmt.Errorf("not connected")
	}

	nodes := make([]*ua.ReadValueID, len(tags))
	for i, t := range tags {
		nid, err := toUANodeID(t.Parsed)
		if err != nil {
			return err
		}
		nodes[i] = &ua.ReadValueID{NodeID: nid}
	}
	req := &ua.ReadRequest{
		MaxAge:             0,
		NodesToRead:        nodes,
		TimestampsToReturn: ua.TimestampsToReturnBoth,
	}
	resp, err := c.Read(ctx, req)
	if err != nil {
		return err
	}
	d.alive.Store(true)
	now := time.Now().UTC()
	for i, rv := range resp.Results {
		s, err := mapDataValue(tags[i], rv, now)
		if err != nil {
			d.log.Warn("skip tag value", "tag", tags[i].ID, "err", err)
			out <- core.Sample{Time: now, TagID: tags[i].ID, Quality: core.QualityBad}
			continue
		}
		select {
		case out <- s:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func toUANodeID(p core.ParsedNodeID) (*ua.NodeID, error) {
	switch p.IdentifierType {
	case "i":
		var id uint32
		_, err := fmt.Sscanf(p.Identifier, "%d", &id)
		if err != nil {
			return nil, err
		}
		return ua.NewNumericNodeID(p.Namespace, id), nil
	case "s":
		return ua.NewStringNodeID(p.Namespace, p.Identifier), nil
	default:
		return nil, fmt.Errorf("identifier type %q not supported in M1", p.IdentifierType)
	}
}
