package rollup

import (
	"testing"
	"time"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
	"github.com/tamcore/garminstatus/pkg/store"
)

func feat(s garminstatus.ServiceStatus, reasons ...string) garminstatus.ServiceMap {
	return garminstatus.ServiceMap{"F": {Status: s, StatusReason: reasons}}
}

// fsnap builds a features-only snapshot.
func fsnap(ts time.Time, k store.Kind, f garminstatus.ServiceMap) store.Snapshot {
	return store.Snapshot{TS: ts, Kind: k, Platforms: ef(), Features: f}
}

// Covers the features category path and uniqueReasons edge cases (empty and
// duplicate reason strings collapsing to nil).
func TestBuildFeaturesAndReasons(t *testing.T) {
	d1 := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	snaps := []store.Snapshot{
		fsnap(d1, store.KindChange, feat(garminstatus.Up)),
		fsnap(d1.Add(time.Hour), store.KindChange, feat(garminstatus.Down, "", "")),
		fsnap(d1.Add(5*time.Hour), store.KindChange, feat(garminstatus.Up)),
	}
	got := Build(snaps)
	if len(got.Services.Features) != 1 || got.Services.Features[0].Name != "F" {
		t.Fatalf("expected feature F, got %+v", got.Services.Features)
	}
	if len(got.Incidents) != 1 {
		t.Fatalf("expected 1 feature incident, got %d", len(got.Incidents))
	}
	if got.Incidents[0].Reasons != nil {
		t.Errorf("empty reasons should collapse to nil, got %v", got.Incidents[0].Reasons)
	}
}
