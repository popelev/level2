package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/popelev/level2/internal/api"
	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	"github.com/popelev/level2/internal/driver/mock"
	"github.com/popelev/level2/internal/historian/timescale"
	devruntime "github.com/popelev/level2/internal/runtime"
	"github.com/popelev/level2/internal/spool"
	"github.com/popelev/level2/internal/store"
)

// processReady mirrors /readyz: sim/full-sample mode or any OPC device connected.
func processReady(fullSamplesSim bool, anyConnected bool) bool {
	return fullSamplesSim || anyConnected
}

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to collector config")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	cfgStore := config.NewStore(*cfgPath, cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	hist, err := timescale.New(ctx, cfg.Database.URL)
	if err != nil {
		log.Error("historian", "err", err)
		os.Exit(1)
	}
	defer hist.Close(context.Background())
	hist.SetCapacityPolicy(cfg.Database.CapacityPercent, cfg.Database.FullPolicy)
	if err := hist.EnsureSchema(ctx); err != nil {
		log.Error("schema", "err", err)
		os.Exit(1)
	}

	sp, err := spool.New(cfg.SpoolDir, 5000)
	if err != nil {
		log.Error("spool", "err", err)
		os.Exit(1)
	}

	live := store.NewLive()
	diagBuf := diag.NewBuffer(3000)
	diag.SetDefault(diagBuf)
	incidents := diag.NewIncidentTracker(4096, time.Hour)
	diag.SetDefaultIncidents(incidents)
	wsHub := api.NewHub()
	raw := make(chan core.Sample, 2048)
	toHist := make(chan core.Sample, 2048)
	go api.FanIn(ctx, raw, live, wsHub, toHist)

	useSim := os.Getenv("LEVEL2_SIM_BROWSER") == "1" || os.Getenv("LEVEL2_SIM_BROWSER") == "true"
	// Legacy global master (config/env). NEVER auto-enabled on OPC disconnect.
	// Prefer per-tag Tag.Simulate — real OPC continues for non-simulated tags.
	globalTagSim := cfg.TagSimulation
	fullSamplesSim := useSim || globalTagSim
	devHub := devruntime.NewHub(log, useSim)
	var collectOnce sync.Map // deviceID -> true

	startCollect := func(deviceID string) {
		startDeviceCollect(ctx, log, cfgStore, devHub, &collectOnce, fullSamplesSim, raw, deviceID)
	}

	for _, dev := range cfgStore.Devices() {
		if err := devHub.Upsert(ctx, dev); err != nil {
			log.Error("device hub", "device", dev.ID, "err", err)
		}
		startCollect(dev.ID)
	}

	demo := mock.NewDemo(time.Second)
	if err := demo.Connect(ctx); err != nil {
		log.Error("demo connect", "err", err)
		os.Exit(1)
	}
	if fullSamplesSim {
		go runDemo(ctx, log, demo, cfgStore, raw, true)
		if useSim {
			log.Info("PLC-off demo mode: multi-server sim address space + synthetic samples")
		} else {
			log.Info("legacy global tag_simulation: synthetic samples for all tags; real OPC collect paused")
		}
	} else {
		// Per-tag simulation: mock only tags with simulate=true; OPC collect stays up for the rest.
		go runDemo(ctx, log, demo, cfgStore, raw, false)
		log.Info("per-tag simulation ready (opt-in via tag.simulate); real OPC collect active")
	}

	go flushLoop(ctx, log, hist, sp, toHist)
	go replaySpool(ctx, log, hist, sp)

	apiSrv := &api.Server{
		Log:       log,
		Live:      live,
		Hub:       wsHub,
		Diag:      diagBuf,
		Incidents: incidents,
		DevHub:    devHub,
		History:   hist,
		DB:        hist,
		Cfg:       cfgStore,
		Tags:      cfgStore.AllTags,
		Devices:   cfgStore.Devices,
		ReadyCheck: func() bool {
			return processReady(fullSamplesSim, devHub.AnyConnected())
		},
		OPCWriteEnabled:     cfgStore.OPCWriteEnabled,
		TagSimulationActive: func() bool { return fullSamplesSim },
		SimBrowserActive:    func() bool { return useSim },
		APIToken:            cfgStore.APIToken,
		APITokenWrite:       cfgStore.APITokenWrite,
		APITokenAdmin:       cfgStore.APITokenAdmin,
		OnDeviceChanged: func(deviceID string, removed bool) {
			if removed {
				devHub.Remove(ctx, deviceID)
				collectOnce.Delete(deviceID)
				return
			}
			for _, d := range cfgStore.Devices() {
				if d.ID == deviceID {
					_ = devHub.Upsert(ctx, d)
					startCollect(deviceID)
					break
				}
			}
		},
	}

	go watchReady(ctx, apiSrv.ReadyCheck)

	srv := &http.Server{Addr: cfg.Listen, Handler: wireHTTP(log, cfg.UIDir, apiSrv, fullSamplesSim, devHub)}
	go func() {
		log.Info("http listen", "addr", cfg.Listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	_ = srv.Shutdown(shutdownCtx)
	for _, d := range devHub.Drivers() {
		_ = d.Disconnect(shutdownCtx)
	}
	log.Info("collector stopped")
}
