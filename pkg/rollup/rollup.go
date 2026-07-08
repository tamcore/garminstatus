// Package rollup reconstructs uptime history from a store change-log.
//
// The change-log records the full service state only at transitions (plus
// heartbeats). Build integrates the state between transitions to produce
// per-service daily up-fractions and a list of incidents. It is a pure
// function of its input so the generated artifact changes only when the
// underlying data changes.
package rollup

import (
	"sort"
	"time"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
	"github.com/tamcore/garminstatus/pkg/store"
)

// DayBucket is one calendar day (UTC) of uptime for a service.
type DayBucket struct {
	Date   string  `json:"date"`   // YYYY-MM-DD (UTC)
	UpFrac float64 `json:"upFrac"` // fraction of covered time the service was up, 0..1
	Worst  string  `json:"worst"`  // "up" or "down" — worst status seen that day
}

// Service is a single Garmin service's current state and daily history.
type Service struct {
	Name    string      `json:"name"`
	Current string      `json:"current"`
	Days    []DayBucket `json:"days"`
}

// Incident is a contiguous down interval for a service. End is nil when the
// service was still down at the end of the data.
type Incident struct {
	Service string     `json:"service"`
	Start   time.Time  `json:"start"`
	End     *time.Time `json:"end,omitempty"`
	Reasons []string   `json:"reasons,omitempty"`
}

// Services groups the two Garmin categories.
type Services struct {
	Platforms []Service `json:"platforms"`
	Features  []Service `json:"features"`
}

// Status is the artifact the static site reads (site/data/status.json).
type Status struct {
	Generated   time.Time  `json:"generated"`
	DataThrough time.Time  `json:"dataThrough"`
	Services    Services   `json:"services"`
	Incidents   []Incident `json:"incidents"`
}

// incidentMinDuration debounces collector noise: a single missed/failed scrape
// can briefly read as "down" across services. Closed incidents shorter than
// this are treated as noise and omitted from the incident list (they still
// count, negligibly, toward time-weighted uptime). Ongoing incidents are always
// kept regardless of elapsed time.
const incidentMinDuration = 15 * time.Minute

type event struct {
	ts      time.Time
	status  garminstatus.ServiceStatus
	reasons []string
}

// Build integrates the change-log into a Status. Records need not be sorted.
func Build(snaps []store.Snapshot) Status {
	out := Status{
		Services:  Services{Platforms: []Service{}, Features: []Service{}},
		Incidents: []Incident{},
	}
	if len(snaps) == 0 {
		return out
	}

	sorted := make([]store.Snapshot, len(snaps))
	copy(sorted, snaps)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TS.Before(sorted[j].TS) })

	dataThrough := sorted[len(sorted)-1].TS.UTC()
	out.DataThrough = dataThrough
	out.Generated = dataThrough // deterministic: no wall-clock churn

	platforms := timelines(sorted, func(s store.Snapshot) garminstatus.ServiceMap { return s.Platforms })
	features := timelines(sorted, func(s store.Snapshot) garminstatus.ServiceMap { return s.Features })

	out.Services.Platforms = services(platforms, dataThrough)
	out.Services.Features = services(features, dataThrough)
	out.Incidents = append(incidents(platforms), incidents(features)...)
	sort.Slice(out.Incidents, func(i, j int) bool { return out.Incidents[i].Start.Before(out.Incidents[j].Start) })
	return out
}

// timelines extracts a per-service ordered event list for one category.
func timelines(sorted []store.Snapshot, pick func(store.Snapshot) garminstatus.ServiceMap) map[string][]event {
	tl := map[string][]event{}
	for _, snap := range sorted {
		for name, info := range pick(snap) {
			tl[name] = append(tl[name], event{ts: snap.TS.UTC(), status: info.Status, reasons: info.StatusReason})
		}
	}
	return tl
}

func serviceNames(tl map[string][]event) []string {
	names := make([]string, 0, len(tl))
	for n := range tl {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

type dayAcc struct {
	cover float64
	up    float64
	down  bool
}

func services(tl map[string][]event, dataThrough time.Time) []Service {
	out := []Service{}
	for _, name := range serviceNames(tl) {
		evs := tl[name]
		acc := map[string]*dayAcc{}
		for i, ev := range evs {
			end := dataThrough
			if i+1 < len(evs) {
				end = evs[i+1].ts
			}
			addInterval(acc, ev.ts, end, ev.status == garminstatus.Up)
		}
		out = append(out, Service{
			Name:    name,
			Current: string(evs[len(evs)-1].status),
			Days:    buckets(acc),
		})
	}
	return out
}

// addInterval attributes [a, b) to per-UTC-day accumulators, splitting on day
// boundaries.
func addInterval(acc map[string]*dayAcc, a, b time.Time, isUp bool) {
	a, b = a.UTC(), b.UTC()
	for a.Before(b) {
		nextDay := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
		segEnd := nextDay
		if b.Before(segEnd) {
			segEnd = b
		}
		key := a.Format("2006-01-02")
		d := acc[key]
		if d == nil {
			d = &dayAcc{}
			acc[key] = d
		}
		dur := segEnd.Sub(a).Seconds()
		d.cover += dur
		if isUp {
			d.up += dur
		} else {
			d.down = true
		}
		a = segEnd
	}
}

func buckets(acc map[string]*dayAcc) []DayBucket {
	keys := make([]string, 0, len(acc))
	for k := range acc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]DayBucket, 0, len(keys))
	for _, k := range keys {
		d := acc[k]
		frac := 1.0
		if d.cover > 0 {
			frac = d.up / d.cover
		}
		worst := string(garminstatus.Up)
		if d.down {
			worst = string(garminstatus.Down)
		}
		out = append(out, DayBucket{Date: k, UpFrac: frac, Worst: worst})
	}
	return out
}

// incidents collapses each service timeline to status transitions and emits one
// Incident per contiguous down interval.
func incidents(tl map[string][]event) []Incident {
	out := []Incident{}
	for _, name := range serviceNames(tl) {
		reduced := collapse(tl[name])
		for i, ev := range reduced {
			if ev.status != garminstatus.Down {
				continue
			}
			inc := Incident{Service: name, Start: ev.ts, Reasons: uniqueReasons(ev.reasons)}
			if i+1 < len(reduced) {
				end := reduced[i+1].ts
				if end.Sub(ev.ts) < incidentMinDuration {
					continue // debounce: too short to be a real outage
				}
				inc.End = &end
			}
			// else: still down at end of data -> End stays nil (ongoing)
			out = append(out, inc)
		}
	}
	return out
}

// collapse removes consecutive events that do not change status (e.g.
// heartbeats), keeping the first occurrence and its reasons.
func collapse(evs []event) []event {
	out := make([]event, 0, len(evs))
	for _, ev := range evs {
		if len(out) > 0 && out[len(out)-1].status == ev.status {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func uniqueReasons(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := []string{}
	for _, r := range in {
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
