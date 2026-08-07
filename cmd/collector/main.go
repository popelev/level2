package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/popelev/level2/internal/api"
	"github.com/popelev/level2/internal/backoff"
	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	"github.com/popelev/level2/internal/driver/mock"
	opcuaDriver "github.com/popelev/level2/internal/driver/opcua"
	"github.com/popelev/level2/internal/historian/timescale"
	"github.com/popelev/level2/internal/metrics"
	devruntime "github.com/popelev/level2/internal/runtime"
	"github.com/popelev/level2/internal/spool"
	"github.com/popelev/level2/internal/store"
)

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
	devHub := devruntime.NewHub(log, useSim)
	var collectOnce sync.Map // deviceID -> true

	startCollect := func(deviceID string) {
		if useSim {
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
		go runDevice(ctx, log, cfgStore, deviceID, drv, raw)
	}

	for _, dev := range cfgStore.Devices() {
		if err := devHub.Upsert(ctx, dev); err != nil {
			log.Error("device hub", "device", dev.ID, "err", err)
		}
		startCollect(dev.ID)
	}

	if useSim {
		demo := mock.NewDemo(time.Second)
		if err := demo.Connect(ctx); err != nil {
			log.Error("demo connect", "err", err)
			os.Exit(1)
		}
		go runDemo(ctx, log, demo, cfgStore, raw)
		log.Info("PLC-off demo mode: multi-server sim address space + synthetic samples")
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
			return useSim || devHub.AnyConnected()
		},
		OPCWriteEnabled: cfgStore.OPCWriteEnabled,
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if useSim || devHub.AnyConnected() {
			metrics.OPCConnected.Set(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ready"))
			return
		}
		metrics.OPCConnected.Set(0)
		http.Error(w, "not connected", http.StatusServiceUnavailable)
	})
	mux.Handle("/metrics", metrics.Handler())
	apiSrv.Mount(mux)
	if st, err := os.Stat(cfg.UIDir); err == nil && st.IsDir() {
		mux.Handle("/", http.FileServer(http.Dir(cfg.UIDir)))
		log.Info("serving ui", "dir", cfg.UIDir)
	}

	srv := &http.Server{Addr: cfg.Listen, Handler: mux}
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

func runDemo(ctx context.Context, log *slog.Logger, demo *mock.Driver, cfgStore *config.Store, samples chan<- core.Sample) {
	for {
		if ctx.Err() != nil {
			return
		}
		gen := cfgStore.Gen()
		tags := cfgStore.AllTags()
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

// watchReady records collector becoming not-ready (mirrors /readyz).
func watchReady(ctx context.Context, ready func() bool) {
	if ready == nil {
		return
	}
	was := ready()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			now := ready()
			if was && !now {
				diag.RecordCollectorDown()
			}
			was = now
		}
	}
}

func flushLoop(ctx context.Context, log *slog.Logger, hist core.Historian, sp *spool.FileSpool, in <-chan core.Sample) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	buf := make([]core.Sample, 0, 256)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		batch := buf
		buf = make([]core.Sample, 0, 256)
		if err := hist.WriteBatch(ctx, batch); err != nil {
			if errors.Is(err, timescale.ErrCapacityHalt) {
				log.Warn("write batch halted by capacity policy", "err", err, "n", len(batch))
				diag.DBWrite(diag.LevelWarn, "historian halted by capacity policy", err.Error(), len(batch))
				return
			}
			metrics.WriteErrors.Inc()
			diag.RecordDBWriteError()
			log.Error("write batch", "err", err, "n", len(batch))
			diag.DBWrite(diag.LevelError, "historian write batch failed", err.Error(), len(batch))
			if serr := sp.Enqueue(batch); serr != nil {
				log.Error("spool enqueue", "err", serr)
				diag.DBWrite(diag.LevelError, "spool enqueue failed", serr.Error(), len(batch))
			} else {
				metrics.SamplesSpooled.Add(float64(len(batch)))
				metrics.SpoolDepth.Set(float64(sp.Len()))
				diag.DBWrite(diag.LevelWarn, "samples spooled to disk", "", len(batch))
			}
			return
		}
		metrics.SamplesWritten.Add(float64(len(batch)))
		diag.DBWrite(diag.LevelInfo, "wrote batch to timescale", "", len(batch))
	}
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case s := <-in:
			buf = append(buf, s)
			if len(buf) >= 200 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func replaySpool(ctx context.Context, log *slog.Logger, hist core.Historian, sp *spool.FileSpool) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			files, err := sp.List()
			if err != nil || len(files) == 0 {
				metrics.SpoolDepth.Set(0)
				continue
			}
			metrics.SpoolDepth.Set(float64(len(files)))
			path := files[0]
			for _, f := range files[1:] {
				if filepath.Base(f) < filepath.Base(path) {
					path = f
				}
			}
			batch, err := sp.Load(path)
			if err != nil {
				log.Warn("spool load", "path", path, "err", err)
				_ = sp.Remove(path)
				continue
			}
			if err := hist.WriteBatch(ctx, batch); err != nil {
				if errors.Is(err, timescale.ErrCapacityHalt) {
					log.Warn("spool replay halted by capacity policy", "err", err)
					diag.DBWrite(diag.LevelWarn, "spool replay halted by capacity policy", err.Error(), len(batch))
					continue
				}
				log.Warn("spool replay failed", "err", err)
				diag.RecordDBWriteError()
				diag.DBWrite(diag.LevelError, "spool replay write failed", err.Error(), len(batch))
				continue
			}
			metrics.SamplesWritten.Add(float64(len(batch)))
			_ = sp.Remove(path)
			metrics.SpoolDepth.Set(float64(sp.Len()))
			log.Info("spool replayed", "n", len(batch), "path", filepath.Base(path))
			diag.DBWrite(diag.LevelInfo, "spool replayed to timescale", filepath.Base(path), len(batch))
		}
	}
}
