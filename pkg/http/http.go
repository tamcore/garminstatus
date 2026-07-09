// Package http serves the daemon's observability endpoints: Prometheus metrics
// and liveness/readiness probes. Metrics are updated by the daemon loop, not on
// scrape, so this server only exposes the current registry.
package http

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/handlers"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/tamcore/garminstatus/pkg/healthcheck"
)

// Handler builds the observability mux exposing /metrics, /live and /ready,
// wrapped in access logging.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	health := healthcheck.Setup()
	mux.HandleFunc("/ready", health.ReadyEndpoint)
	mux.HandleFunc("/live", health.LiveEndpoint)

	return handlers.CombinedLoggingHandler(os.Stdout, mux)
}

// Serve starts the HTTP server on addr (e.g. ":8080") and blocks.
func Serve(addr string) error {
	server := &http.Server{
		Addr:              addr,
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           Handler(),
	}

	fmt.Println("serving metrics/health on", addr)
	if err := server.ListenAndServe(); err != nil {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}
