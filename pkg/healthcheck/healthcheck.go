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

func Setup() healthcheck.Handler {
	handler := healthcheck.NewHandler()

	// Add a readiness check to make sure an upstream dependency resolves in DNS.
	handler.AddReadinessCheck("upstream-dep-dns", dns.Resolve(func() string {
		u, err := url.Parse(garminstatus.GarminConnectStatusURI)
		if err != nil {
			log.Fatal(err)
		}
		return u.Hostname()
	}(), 50*time.Millisecond))

	// Add a liveness check to detect Goroutine leaks. If this fails we want to be restarted/rescheduled.
	handler.AddLivenessCheck("goroutine-threshold", goroutine.Count(100))

	return handler
}
