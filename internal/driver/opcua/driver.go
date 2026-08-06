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
	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
)

// Driver is an OPC UA client that polls leaf nodes (M1).
// Structure nodes are rejected at value-map time — expand comes in M2.
type Driver struct {
	device core.Device
	log    *slog.Logger

	mu      sync.Mutex
	client  *opcua.Client
	alive   atomic.Bool
	nsCache map[string]uint16
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

	config.ApplyDeviceEnvOverlay(&d.device)

	endpoints, err := opcua.GetEndpoints(ctx, d.device.Endpoint)
	if err != nil {
		return fmt.Errorf("opcua get endpoints: %w", err)
	}
	ep, err := opcua.SelectEndpoint(endpoints, ua.SecurityPolicyURINone, ua.MessageSecurityModeNone)
	if err != nil {
		return fmt.Errorf("opcua select endpoint: %w", err)
	}

	opts := []opcua.Option{
		opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeUserName),
	}
	if d.device.Username != "" {
		opts = append(opts, opcua.AuthUsername(d.device.Username, d.device.Password))
	} else {
		opts = append(opts, opcua.AuthAnonymous())
	}

	c, err := opcua.NewClient(ep.EndpointURL, opts...)
	if err != nil {
		return fmt.Errorf("opcua new client: %w", err)
	}
	if err := c.Connect(ctx); err != nil {
		return fmt.Errorf("opcua connect %s: %w", ep.EndpointURL, err)
	}
	d.client = c
	d.alive.Store(true)
	d.nsCache = nil
	d.log.Info("opcua connected", "endpoint", d.device.Endpoint, "device", d.device.ID, "user", d.device.Username)
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

	// Multi-tag: poll all enabled tags on the shortest interval (M2).
	interval := time.Duration(enabled[0].IntervalMs) * time.Millisecond
	for _, t := range enabled[1:] {
		iv := time.Duration(t.IntervalMs) * time.Millisecond
		if iv > 0 && iv < interval {
			interval = iv
		}
	}
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
				diag.OPCRead(diag.LevelWarn, d.device.ID, "", "opc poll failed", err.Error())
				d.alive.Store(false)
				_ = d.Disconnect(ctx)
				if cerr := d.Connect(ctx); cerr != nil {
					d.log.Error("opcua reconnect failed", "err", cerr)
					diag.OPCRead(diag.LevelError, d.device.ID, "", "opc reconnect failed", cerr.Error())
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
	case core.ValueBool, core.ValueInt64, core.ValueFloat64, core.ValueString, core.ValueDateTime:
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

// Siemens and other servers reject very large Read requests (BadTooManyOperations).
const maxNodesPerRead = 100

func (d *Driver) pollOnce(ctx context.Context, tags []TagView, out chan<- core.Sample) error {
	for start := 0; start < len(tags); start += maxNodesPerRead {
		end := start + maxNodesPerRead
		if end > len(tags) {
			end = len(tags)
		}
		if err := d.pollBatch(ctx, tags[start:end], out); err != nil {
			return err
		}
	}
	return nil
}

func (d *Driver) pollBatch(ctx context.Context, tags []TagView, out chan<- core.Sample) error {
	d.mu.Lock()
	c := d.client
	d.mu.Unlock()
	if c == nil {
		return fmt.Errorf("not connected")
	}

	nodes := make([]*ua.ReadValueID, len(tags))
	for i, t := range tags {
		nid, err := d.toUANodeID(ctx, t.Parsed)
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
			diag.OPCRead(diag.LevelWarn, d.device.ID, tags[i].ID, "skip tag read", err.Error())
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

func (d *Driver) toUANodeID(ctx context.Context, p core.ParsedNodeID) (*ua.NodeID, error) {
	ns := p.Namespace
	if p.NamespaceURI != "" {
		idx, err := d.namespaceIndex(ctx, p.NamespaceURI)
		if err != nil {
			return nil, err
		}
		ns = idx
	}
	switch p.IdentifierType {
	case "i":
		var id uint32
		_, err := fmt.Sscanf(p.Identifier, "%d", &id)
		if err != nil {
			return nil, err
		}
		return ua.NewNumericNodeID(ns, id), nil
	case "s":
		return ua.NewStringNodeID(ns, p.Identifier), nil
	default:
		return nil, fmt.Errorf("identifier type %q not supported", p.IdentifierType)
	}
}

func (d *Driver) namespaceIndex(ctx context.Context, uri string) (uint16, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.nsCache != nil {
		if idx, ok := d.nsCache[uri]; ok {
			return idx, nil
		}
	}
	if d.client == nil {
		return 0, fmt.Errorf("not connected")
	}
	arr, err := d.client.NamespaceArray(ctx)
	if err != nil {
		return 0, fmt.Errorf("namespace array: %w", err)
	}
	d.nsCache = make(map[string]uint16, len(arr))
	for i, u := range arr {
		d.nsCache[u] = uint16(i)
	}
	idx, ok := d.nsCache[uri]
	if !ok {
		return 0, fmt.Errorf("namespace uri %q not found on server", uri)
	}
	return idx, nil
}

func toUANodeID(p core.ParsedNodeID) (*ua.NodeID, error) {
	// used by browse helpers without URI resolution
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
