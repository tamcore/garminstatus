package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
)

var (
	serveHTTP bool
	httpPort  int
)

var (
    platformStatus = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Namespace: "garmin",
            Subsystem: "platform",
            Name:      "status",
            Help:      "Garmin platform status (1 for up, 0 for down)",
        },
        []string{"service"},
    )

    featureStatus = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Namespace: "garmin",
            Subsystem: "feature",
            Name:      "status",
            Help:      "Garmin feature status (1 for up, 0 for down)",
        },
        []string{"service"},
    )
)

func init() {
	flag.BoolVar(&serveHTTP, "http", false, "Start an HTTP server to serve the JSON data")
	flag.IntVar(&httpPort, "port", 8080, "HTTP server port")
	flag.Parse()

    prometheus.MustRegister(platformStatus)
    prometheus.MustRegister(featureStatus)
}

func main() {
	if serveHTTP {
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
			w.Write(jsonData)
		})
		metricsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			updateMetrics()
			promhttp.Handler().ServeHTTP(w, r)
		})

		// Expose a Prometheus /metrics endpoint with the custom handler
		http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			metricsHandler.ServeHTTP(w, r)
		})

		fmt.Println("Starting HTTP server on port", httpPort)
		err := http.ListenAndServe(fmt.Sprintf(":%d", httpPort), nil)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		// Fetch Garmin service status and output to stdout when not serving via HTTP
		status, err := garminstatus.FetchStatus()
		if err != nil {
			log.Fatal(err)
		}

		// Convert the status to JSON
		jsonData, err := json.Marshal(status)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(string(jsonData))
	}
}

// updateMetrics fetches Garmin service status and updates Prometheus metrics.
func updateMetrics() {
	// Fetch Garmin service status
	status, err := garminstatus.FetchStatus()
	if err != nil {
		log.Println("Error fetching Garmin status:", err)
	}

	// Update Prometheus metrics based on the fetched status
	updateMetricsOnce(status)
}

// updateMetricsOnce updates Prometheus metrics based on the Garmin service status.
func updateMetricsOnce(status garminstatus.GarminStatus) {
	// Reset metrics before updating to avoid duplicates
	platformStatus.Reset()
	featureStatus.Reset()

    // Set Prometheus metrics for platform and feature statuses
    for service, s := range status.Platforms {
        if s == garminstatus.Up {
            platformStatus.WithLabelValues(service).Set(1)
        } else {
            platformStatus.WithLabelValues(service).Set(0)
        }
    }

    for service, s := range status.Features {
        if s == garminstatus.Up {
            featureStatus.WithLabelValues(service).Set(1)
        } else {
            featureStatus.WithLabelValues(service).Set(0)
        }
    }
}

// mapStatusToFloat maps Garmin service status to float values (1: up, 0: down).
func mapStatusToFloat(status garminstatus.ServiceStatus) int {
	if status == garminstatus.Up {
		return 1
	}
	return 0
}
