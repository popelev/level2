package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/popelev/level2/internal/api"
	"github.com/popelev/level2/internal/backoff"
	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
	opcuaDriver "github.com/popelev/level2/internal/driver/opcua"
	"github.com/popelev/level2/internal/driver/simbrowser"
	"github.com/popelev/level2/internal/historian/timescale"
	"github.com/popelev/level2/internal/metrics"
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

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	hist, err := timescale.New(ctx, cfg.Database.URL)
	if err != nil {
		log.Error("historian", "err", err)
		os.Exit(1)
	}
	defer hist.Close(context.Background())
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
	hub := api.NewHub()
	raw := make(chan core.Sample, 2048)
	toHist := make(chan core.Sample, 2048)
	go api.FanIn(ctx, raw, live, hub, toHist)

	var drivers []core.Driver
	var primaryBrowser core.Browser
	useSim := os.Getenv("LEVEL2_SIM_BROWSER") == "1" || os.Getenv("LEVEL2_SIM_BROWSER") == "true"
	if useSim {
		primaryBrowser = simbrowser.NewDemo()
		log.Info("using in-memory OPC sim browser (PLC-off)")
	}
	for _, dev := range cfg.Devices {
		d := opcuaDriver.New(dev, log.With("device", dev.ID))
		if err := d.Connect(ctx); err != nil {
			log.Error("opc connect", "device", dev.ID, "err", err)
		}
		if primaryBrowser == nil {
			primaryBrowser = d
		}
		drivers = append(drivers, d)
		devCopy := dev
		go runDevice(ctx, log, devCopy, d, raw)
	}

	go flushLoop(ctx, log, hist, sp, toHist)
	go replaySpool(ctx, log, hist, sp)

	apiSrv := &api.Server{
		Log:     log,
		Live:    live,
		Hub:     hub,
		History: hist,
		Browser: primaryBrowser,
		Tags: func() []core.Tag {
			var tags []core.Tag
			for _, d := range cfg.Devices {
				tags = append(tags, d.Tags...)
			}
			return tags
		},
		Devices: func() []core.Device { return cfg.Devices },
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		for _, d := range drivers {
			if d.Connected() {
				metrics.OPCConnected.Set(1)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ready"))
				return
			}
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
	for _, d := range drivers {
		_ = d.Disconnect(shutdownCtx)
	}
	log.Info("collector stopped")
}

func runDevice(ctx context.Context, log *slog.Logger, device core.Device, drv *opcuaDriver.Driver, samples chan<- core.Sample) {
	bo := backoff.New(2*time.Second, 30*time.Second)
	for {
		if ctx.Err() != nil {
			return
		}
		if !drv.Connected() {
			if err := drv.Connect(ctx); err != nil {
				wait := bo.Next()
				log.Warn("waiting opc", "device", device.ID, "err", err, "backoff", wait.String())
				select {
				case <-ctx.Done():
					return
				case <-time.After(wait):
					continue
				}
			}
			bo.Reset()
		}
		err := drv.Subscribe(ctx, device.Tags, samples)
		if ctx.Err() != nil {
			return
		}
		log.Warn("subscribe ended", "device", device.ID, "err", err)
		_ = drv.Disconnect(ctx)
		wait := bo.Next()
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
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
			metrics.WriteErrors.Inc()
			log.Error("write batch", "err", err, "n", len(batch))
			if serr := sp.Enqueue(batch); serr != nil {
				log.Error("spool enqueue", "err", serr)
			} else {
				metrics.SamplesSpooled.Add(float64(len(batch)))
				metrics.SpoolDepth.Set(float64(sp.Len()))
			}
			return
		}
		metrics.SamplesWritten.Add(float64(len(batch)))
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
			// oldest first by name (nanosecond timestamps)
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
				log.Warn("spool replay failed", "err", err)
				continue
			}
			metrics.SamplesWritten.Add(float64(len(batch)))
			_ = sp.Remove(path)
			metrics.SpoolDepth.Set(float64(sp.Len()))
			log.Info("spool replayed", "n", len(batch), "path", filepath.Base(path))
		}
	}
}
