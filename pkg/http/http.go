package http

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/handlers"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
	"github.com/tamcore/garminstatus/pkg/healthcheck"
	"github.com/tamcore/garminstatus/pkg/metrics"
)

func ServeHTTP(port int) error {
	// Start an HTTP server if the -http flag is provided
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Fetch Garmin service status on each request when serving via HTTP
		status, err := garminstatus.FetchStatus()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Convert the status to JSON
		jsonData, err := json.Marshal(status)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(jsonData)
		if err != nil {
			log.Fatal(err)
		}
	})
	metricsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metrics.UpdateMetrics()
		promhttp.Handler().ServeHTTP(w, r)
	})

	// Expose a Prometheus /metrics endpoint with the custom handler
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metricsHandler.ServeHTTP(w, r)
	})

	healthHandler := healthcheck.Setup()
	http.HandleFunc("/ready", healthHandler.ReadyEndpoint)
	http.HandleFunc("/live", healthHandler.LiveEndpoint)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           handlers.CombinedLoggingHandler(os.Stdout, http.DefaultServeMux),
	}

	fmt.Println("Starting HTTP server on port", port)
	err := httpServer.ListenAndServe()
	if err != nil {
		return fmt.Errorf("failed to start http server: %w", err)
	}
	return nil
}
