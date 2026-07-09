// Package metrics exposes Prometheus metrics for the daemon: the Garmin service
// status gauges plus counters/gauges tracking the git pull/push and publish
// cycle so the collector's own health is observable.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
)

const (
	namespace   = "garminstatus"
	labelBranch = "branch"
	labelOp     = "op"
)

var (
	platformStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "garmin", Subsystem: "platform", Name: "status",
		Help: "Garmin platform status (1 for up, 0 for down)",
	}, []string{"service"})

	featureStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "garmin", Subsystem: "feature", Name: "status",
		Help: "Garmin feature status (1 for up, 0 for down)",
	}, []string{"service"})

	fetchSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace, Name: "fetch_success",
		Help: "Whether the last Garmin status fetch succeeded (1) or not (0)",
	})

	syncSuccess = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace, Name: "sync_success",
		Help: "Whether the last git operation succeeded (1) or not (0)",
	}, []string{labelBranch, labelOp})

	syncTimestamp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace, Name: "sync_timestamp_seconds",
		Help: "Unix time of the last successful git operation",
	}, []string{labelBranch, labelOp})

	syncErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "sync_errors_total",
		Help: "Total number of failed git operations",
	}, []string{labelBranch, labelOp})

	cycleTimestamp = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace, Name: "cycle_timestamp_seconds",
		Help: "Unix time of the last completed collect/publish cycle",
	})
)

func init() {
	prometheus.MustRegister(
		platformStatus, featureStatus,
		fetchSuccess, syncSuccess, syncTimestamp, syncErrors, cycleTimestamp,
	)
}

// UpdateStatus reflects a fetched Garmin status into the status gauges.
func UpdateStatus(status garminstatus.GarminStatus) {
	platformStatus.Reset()
	featureStatus.Reset()
	for service, s := range status.Platforms {
		platformStatus.WithLabelValues(service).Set(boolToFloat(s.Status == garminstatus.Up))
	}
	for service, s := range status.Features {
		featureStatus.WithLabelValues(service).Set(boolToFloat(s.Status == garminstatus.Up))
	}
}

// RecordFetch records whether a Garmin fetch succeeded.
func RecordFetch(ok bool) { fetchSuccess.Set(boolToFloat(ok)) }

// RecordSync records the outcome of a git operation (branch + op such as
// "pull"/"push"). On success it stamps the timestamp; on error it increments
// the error counter. Both set the success gauge to 1/0.
func RecordSync(branch, op string, err error) {
	if err != nil {
		syncSuccess.WithLabelValues(branch, op).Set(0)
		syncErrors.WithLabelValues(branch, op).Inc()
		return
	}
	syncSuccess.WithLabelValues(branch, op).Set(1)
	syncTimestamp.WithLabelValues(branch, op).Set(float64(time.Now().Unix()))
}

// RecordCycle stamps the completion time of a full loop iteration.
func RecordCycle() { cycleTimestamp.Set(float64(time.Now().Unix())) }

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
