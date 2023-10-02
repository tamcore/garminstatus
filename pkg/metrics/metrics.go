package metrics

import (
    "log"

    "github.com/prometheus/client_golang/prometheus"

    "github.com/tamcore/garminstatus/pkg/garminstatus"
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
    prometheus.MustRegister(platformStatus)
    prometheus.MustRegister(featureStatus)
}

// UpdateMetrics fetches Garmin service status and updates Prometheus metrics.
func UpdateMetrics() {
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
        if s.Status == garminstatus.Up {
            platformStatus.WithLabelValues(service).Set(1)
        } else {
            platformStatus.WithLabelValues(service).Set(0)
        }
    }

    for service, s := range status.Features {
        if s.Status == garminstatus.Up {
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
