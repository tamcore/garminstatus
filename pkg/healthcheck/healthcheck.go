// Package healthcheck wires liveness and readiness probes for the daemon.
package healthcheck

import (
	"log"
	"net/url"
	"time"

	"github.com/tamcore/go-healthcheck"
	"github.com/tamcore/go-healthcheck/checks/dns"
	"github.com/tamcore/go-healthcheck/checks/goroutine"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
)

// Setup returns a health handler with a DNS readiness check on the upstream
// Garmin host and a goroutine-leak liveness check.
func Setup() healthcheck.Handler {
	handler := healthcheck.NewHandler()

	// Readiness: the upstream dependency must resolve in DNS.
	handler.AddReadinessCheck("upstream-dep-dns", dns.Resolve(func() string {
		u, err := url.Parse(garminstatus.GarminConnectStatusURI)
		if err != nil {
			log.Fatal(err)
		}
		return u.Hostname()
	}(), 50*time.Millisecond))

	// Liveness: detect goroutine leaks; a failure triggers a restart.
	handler.AddLivenessCheck("goroutine-threshold", goroutine.Count(100))

	return handler
}
