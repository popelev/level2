package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/popelev/level2/internal/api"
	"github.com/popelev/level2/internal/metrics"
	devruntime "github.com/popelev/level2/internal/runtime"
)

// wireHTTP mounts health/ready/metrics, API routes, and optional UI static files.
// Returns the handler wrapped with API auth (same as former inline main wiring).
func wireHTTP(log *slog.Logger, uiDir string, apiSrv *api.Server, fullSamplesSim bool, devHub *devruntime.Hub) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if fullSamplesSim || devHub.AnyConnected() {
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
	if st, err := os.Stat(uiDir); err == nil && st.IsDir() {
		mux.Handle("/", http.FileServer(http.Dir(uiDir)))
		log.Info("serving ui", "dir", uiDir)
	}
	return apiSrv.APIAuth(mux)
}
