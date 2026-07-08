package rollup

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
	"github.com/tamcore/garminstatus/pkg/store"
)

// svc builds a single-service platform map for service "A".
func svc(s garminstatus.ServiceStatus, reasons ...string) garminstatus.ServiceMap {
	return garminstatus.ServiceMap{"A": {Status: s, StatusReason: reasons}}
}

// ef is an empty features map.
func ef() garminstatus.ServiceMap { return garminstatus.ServiceMap{} }

// snap is a terse Snapshot builder for platform-only test cases.
func snap(ts time.Time, k store.Kind, p garminstatus.ServiceMap) store.Snapshot {
	return store.Snapshot{TS: ts, Kind: k, Platforms: p, Features: ef()}
}

func TestBuildEmpty(t *testing.T) {
	got := Build(nil)
	if got.Services.Platforms == nil || got.Services.Features == nil || got.Incidents == nil {
		t.Fatal("empty build must produce non-nil slices for clean JSON")
	}
	b, _ := json.Marshal(got)
	if string(b) == "" {
		t.Fatal("marshal failed")
	}
}

func TestBuildDailyUpFracAndIncident(t *testing.T) {
	// Arrange: A up at d1 00:00, down at d1 12:00 (reason "x"), up again d2 00:00.
	d1 := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	snaps := []store.Snapshot{
		snap(d1, store.KindChange, svc(garminstatus.Up)),
		snap(d1.Add(12*time.Hour), store.KindChange, svc(garminstatus.Down, "x")),
		snap(d1.AddDate(0, 0, 1), store.KindChange, svc(garminstatus.Up)),
	}

	// Act
	got := Build(snaps)

	// Assert
	if len(got.Services.Platforms) != 1 {
		t.Fatalf("expected 1 platform service, got %d", len(got.Services.Platforms))
	}
	a := got.Services.Platforms[0]
	if a.Name != "A" || a.Current != "up" {
		t.Errorf("unexpected service: %+v", a)
	}
	if len(a.Days) != 1 {
		t.Fatalf("expected 1 day bucket (d2 has zero coverage), got %d: %+v", len(a.Days), a.Days)
	}
	if a.Days[0].Date != "2026-01-10" {
		t.Errorf("unexpected day date: %s", a.Days[0].Date)
	}
	if a.Days[0].UpFrac < 0.49 || a.Days[0].UpFrac > 0.51 {
		t.Errorf("expected ~0.5 upFrac, got %f", a.Days[0].UpFrac)
	}
	if a.Days[0].Worst != "down" {
		t.Errorf("expected worst=down, got %s", a.Days[0].Worst)
	}

	if len(got.Incidents) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(got.Incidents))
	}
	inc := got.Incidents[0]
	if inc.Service != "A" || !inc.Start.Equal(d1.Add(12*time.Hour)) {
		t.Errorf("unexpected incident start: %+v", inc)
	}
	if inc.End == nil || !inc.End.Equal(d1.AddDate(0, 0, 1)) {
		t.Errorf("expected incident end at d2 00:00, got %v", inc.End)
	}
	if len(inc.Reasons) != 1 || inc.Reasons[0] != "x" {
		t.Errorf("expected reason [x], got %v", inc.Reasons)
	}
}

func TestBuildDropsShortNoiseIncident(t *testing.T) {
	// A 3-minute down blip (below the debounce threshold) must be dropped from
	// the incident list, while a multi-hour outage is kept.
	d1 := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	snaps := []store.Snapshot{
		snap(d1, store.KindChange, svc(garminstatus.Up)),
		snap(d1.Add(1*time.Hour), store.KindChange, svc(garminstatus.Down)),
		snap(d1.Add(1*time.Hour+3*time.Minute), store.KindChange, svc(garminstatus.Up)),
		snap(d1.Add(5*time.Hour), store.KindChange, svc(garminstatus.Down)),
		snap(d1.Add(9*time.Hour), store.KindChange, svc(garminstatus.Up)),
	}
	got := Build(snaps)
	if len(got.Incidents) != 1 {
		t.Fatalf("expected 1 kept incident (3-min blip dropped), got %d", len(got.Incidents))
	}
	if !got.Incidents[0].Start.Equal(d1.Add(5 * time.Hour)) {
		t.Errorf("kept incident should be the 4h outage, got start %v", got.Incidents[0].Start)
	}
}

func TestBuildHeartbeatDoesNotSplitIncident(t *testing.T) {
	d1 := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	snaps := []store.Snapshot{
		snap(d1, store.KindChange, svc(garminstatus.Down)),
		snap(d1.Add(3*time.Hour), store.KindHeartbeat, svc(garminstatus.Down)),
		snap(d1.Add(6*time.Hour), store.KindHeartbeat, svc(garminstatus.Down)),
	}
	got := Build(snaps)
	if len(got.Incidents) != 1 {
		t.Fatalf("heartbeats should not split the down interval; got %d incidents", len(got.Incidents))
	}
	if got.Incidents[0].End != nil {
		t.Errorf("still-down incident should have nil End (ongoing), got %v", got.Incidents[0].End)
	}
}

func TestBuildUnsortedInputIsSorted(t *testing.T) {
	d1 := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	snaps := []store.Snapshot{
		snap(d1.Add(time.Hour), store.KindChange, svc(garminstatus.Down)),
		snap(d1, store.KindChange, svc(garminstatus.Up)),
	}
	got := Build(snaps)
	if !got.DataThrough.Equal(d1.Add(time.Hour)) {
		t.Errorf("dataThrough should be latest ts, got %v", got.DataThrough)
	}
	if !got.Generated.Equal(got.DataThrough) {
		t.Errorf("generated must equal dataThrough for determinism")
	}
}
