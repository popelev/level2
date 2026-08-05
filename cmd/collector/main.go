package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
	opcuaDriver "github.com/popelev/level2/internal/driver/opcua"
	"github.com/popelev/level2/internal/historian/timescale"
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

	samples := make(chan core.Sample, 1024)
	var drivers []core.Driver
	for _, dev := range cfg.Devices {
		d := opcuaDriver.New(dev, log.With("device", dev.ID))
		if err := d.Connect(ctx); err != nil {
			log.Error("opc connect", "device", dev.ID, "err", err)
			// continue starting HTTP; reconnect loop inside Subscribe
		}
		drivers = append(drivers, d)
		devCopy := dev
		go func(device core.Device, drv *opcuaDriver.Driver) {
			for {
				if !drv.Connected() {
					if err := drv.Connect(ctx); err != nil {
						log.Warn("waiting opc", "device", device.ID, "err", err)
						select {
						case <-ctx.Done():
							return
						case <-time.After(3 * time.Second):
							continue
						}
					}
				}
				err := drv.Subscribe(ctx, device.Tags, samples)
				if ctx.Err() != nil {
					return
				}
				log.Warn("subscribe ended", "device", device.ID, "err", err)
				_ = drv.Disconnect(ctx)
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
			}
		}(devCopy, d)
	}

	go flushLoop(ctx, log, hist, samples)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		for _, d := range drivers {
			if d.Connected() {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ready"))
				return
			}
		}
		http.Error(w, "not connected", http.StatusServiceUnavailable)
	})

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

func flushLoop(ctx context.Context, log *slog.Logger, hist core.Historian, in <-chan core.Sample) {
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
			log.Error("write batch", "err", err, "n", len(batch))
		}
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
