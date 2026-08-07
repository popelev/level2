package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/popelev/level2/internal/backoff"
	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	"github.com/popelev/level2/internal/driver/mock"
	opcuaDriver "github.com/popelev/level2/internal/driver/opcua"
	devruntime "github.com/popelev/level2/internal/runtime"
)

func runDemo(ctx context.Context, log *slog.Logger, demo *mock.Driver, cfgStore *config.Store, samples chan<- core.Sample, allTags bool) {
	for {
		if ctx.Err() != nil {
			return
		}
		gen := cfgStore.Gen()
		tags := selectSimTags(cfgStore.AllTags(), allTags || cfgStore.TagSimulation())
		if len(tags) == 0 {
			subCtx, cancel := context.WithCancel(ctx)
			go watchConfig(subCtx, cancel, cfgStore, gen)
			select {
			case <-ctx.Done():
				cancel()
				return
			case <-subCtx.Done():
				cancel()
			}
			continue
		}
		subCtx, cancel := context.WithCancel(ctx)
		go watchConfig(subCtx, cancel, cfgStore, gen)
		err := demo.Subscribe(subCtx, tags, samples)
		cancel()
		if ctx.Err() != nil {
			return
		}
		log.Info("demo subscribe reload", "err", err, "tags", len(tags))
		time.Sleep(200 * time.Millisecond)
	}
}

func selectSimTags(all []core.Tag, allEnabled bool) []core.Tag {
	out := make([]core.Tag, 0, len(all))
	for _, t := range all {
		if !t.Enabled {
			continue
		}
		if allEnabled || t.Simulate {
			out = append(out, t)
		}
	}
	return out
}

func selectOPCTags(all []core.Tag) []core.Tag {
	out := make([]core.Tag, 0, len(all))
	for _, t := range all {
		if t.Simulate {
			continue
		}
		out = append(out, t)
	}
	return out
}

func runDevice(ctx context.Context, log *slog.Logger, cfgStore *config.Store, deviceID string, drv *opcuaDriver.Driver, samples chan<- core.Sample) {
	bo := backoff.New(2*time.Second, 30*time.Second)
	for {
		if ctx.Err() != nil {
			return
		}
		if !drv.Connected() {
			if err := drv.Connect(ctx); err != nil {
				wait := bo.Next()
				log.Warn("waiting opc", "device", deviceID, "err", err, "backoff", wait.String())
				diag.OPCRead(diag.LevelWarn, deviceID, "", "waiting for opc connection", err.Error())
				select {
				case <-ctx.Done():
					return
				case <-time.After(wait):
					continue
				}
			}
			bo.Reset()
		}
		gen := cfgStore.Gen()
		tags, err := cfgStore.DeviceTags(deviceID)
		if err != nil {
			log.Error("device tags", "err", err)
			return
		}
		tags = selectOPCTags(tags)
		if len(tags) == 0 {
			subCtx, cancel := context.WithCancel(ctx)
			go watchConfig(subCtx, cancel, cfgStore, gen)
			select {
			case <-ctx.Done():
				cancel()
				return
			case <-subCtx.Done():
				cancel()
			}
			continue
		}
		subCtx, cancel := context.WithCancel(ctx)
		go watchConfig(subCtx, cancel, cfgStore, gen)
		err = drv.Subscribe(subCtx, tags, samples)
		cancel()
		if ctx.Err() != nil {
			return
		}
		log.Warn("subscribe ended", "device", deviceID, "err", err)
		if err != nil {
			diag.OPCRead(diag.LevelWarn, deviceID, "", "opc subscribe ended", err.Error())
		}
		// Intentional Disconnect (config reload) — OPC drops are recorded in driver markDown.
		_ = drv.Disconnect(ctx)
		wait := bo.Next()
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func watchConfig(ctx context.Context, cancel context.CancelFunc, cfgStore *config.Store, gen uint64) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if cfgStore.Gen() != gen {
				cancel()
				return
			}
		}
	}
}

// startDeviceCollect launches runDevice once per device when not in full-sample sim mode.
// Extracted from main for unit tests (collectOnce / type-assert / missing entry).
func startDeviceCollect(
	ctx context.Context,
	log *slog.Logger,
	cfgStore *config.Store,
	devHub *devruntime.Hub,
	collectOnce *sync.Map,
	fullSamplesSim bool,
	samples chan<- core.Sample,
	deviceID string,
) {
	if fullSamplesSim {
		return
	}
	if _, loaded := collectOnce.LoadOrStore(deviceID, true); loaded {
		return
	}
	ent, ok := devHub.Entry(deviceID)
	if !ok {
		return
	}
	drv, ok := ent.Driver.(*opcuaDriver.Driver)
	if !ok {
		return
	}
	go runDevice(ctx, log, cfgStore, deviceID, drv, samples)
}
