// Package store persists Garmin status as an append-only change-log (JSONL).
//
// A record is written only when the set of service statuses changes, plus a
// periodic heartbeat so freshness still advances when nothing changes. Uptime
// is later reconstructed by integrating between these transitions.
package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
)

// Kind distinguishes a real status transition from a liveness heartbeat.
type Kind string

const (
	// KindChange marks a record where at least one service status changed.
	KindChange Kind = "change"
	// KindHeartbeat marks a record that repeats the last state for freshness.
	KindHeartbeat Kind = "heartbeat"
)

// Snapshot is one line in the change-log: the full service state at a moment.
type Snapshot struct {
	TS        time.Time               `json:"ts"`
	Kind      Kind                    `json:"kind"`
	Platforms garminstatus.ServiceMap `json:"platforms"`
	Features  garminstatus.ServiceMap `json:"features"`
}

// ReadAll parses every JSONL record from path. A missing file is not an error
// and yields an empty slice.
func ReadAll(path string) ([]Snapshot, error) {
	f, err := os.Open(path) //nolint:gosec // path is operator-controlled, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var out []Snapshot
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var s Snapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("parse %s line %d: %w", path, line, err)
		}
		out = append(out, s)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return out, nil
}

// Last returns the final record in the log, or ok=false if the log is empty.
func Last(path string) (Snapshot, bool, error) {
	all, err := ReadAll(path)
	if err != nil {
		return Snapshot{}, false, err
	}
	if len(all) == 0 {
		return Snapshot{}, false, nil
	}
	return all[len(all)-1], true, nil
}

// Append writes one record as a JSON line, creating the file and parent dir if
// needed.
func Append(path string, s Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir for %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // operator-controlled path
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	enc, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if _, err := f.Write(append(enc, '\n')); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// WriteAll rewrites path with exactly the given records (one JSON line each),
// creating the parent dir if needed. Used to merge/rewrite the log.
func WriteAll(path string, snaps []Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir for %s: %w", path, err)
	}
	f, err := os.Create(path) //nolint:gosec // operator-controlled path
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	w := bufio.NewWriter(f)
	for _, s := range snaps {
		enc, err := json.Marshal(s)
		if err != nil {
			return fmt.Errorf("marshal snapshot: %w", err)
		}
		if _, err := w.Write(append(enc, '\n')); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return w.Flush()
}

// SameStatus reports whether two snapshots have identical service->status
// projections across both categories (reasons ignored).
func SameStatus(a, b Snapshot) bool {
	return sameStatus(a.Platforms, b.Platforms) && sameStatus(a.Features, b.Features)
}

// Collapse sorts records by timestamp, drops exact-timestamp duplicates (keeping
// the last), then removes consecutive records whose status projection is
// unchanged — leaving a minimal transition log.
func Collapse(snaps []Snapshot) []Snapshot {
	if len(snaps) == 0 {
		return nil
	}
	sorted := make([]Snapshot, len(snaps))
	copy(sorted, snaps)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].TS.Before(sorted[j].TS) })

	// Drop exact-timestamp duplicates, keeping the last write for that instant.
	deduped := sorted[:0]
	for i, s := range sorted {
		if i+1 < len(sorted) && sorted[i+1].TS.Equal(s.TS) {
			continue
		}
		deduped = append(deduped, s)
	}

	out := make([]Snapshot, 0, len(deduped))
	for _, s := range deduped {
		if len(out) > 0 && SameStatus(out[len(out)-1], s) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// statusOf projects a ServiceMap to service name -> status, dropping reasons so
// that reason-only text changes do not count as status transitions.
func statusOf(m garminstatus.ServiceMap) map[string]garminstatus.ServiceStatus {
	out := make(map[string]garminstatus.ServiceStatus, len(m))
	for name, info := range m {
		out[name] = info.Status
	}
	return out
}

func sameStatus(a, b garminstatus.ServiceMap) bool {
	pa, pb := statusOf(a), statusOf(b)
	if len(pa) != len(pb) {
		return false
	}
	for k, v := range pa {
		if pb[k] != v {
			return false
		}
	}
	return true
}

// Decide determines whether to write a record for the current status.
//
//   - No prior record: write a change.
//   - Status differs from prior: write a change.
//   - Otherwise, if heartbeat has elapsed since the prior record: write a
//     heartbeat.
//   - Otherwise: no write.
func Decide(
	last *Snapshot,
	current garminstatus.GarminStatus,
	now time.Time,
	heartbeat time.Duration,
) (Snapshot, bool) {
	rec := Snapshot{
		TS:        now.UTC(),
		Kind:      KindChange,
		Platforms: current.Platforms,
		Features:  current.Features,
	}
	if last == nil {
		return rec, true
	}
	if !sameStatus(last.Platforms, current.Platforms) || !sameStatus(last.Features, current.Features) {
		return rec, true
	}
	if heartbeat > 0 && now.UTC().Sub(last.TS) >= heartbeat {
		rec.Kind = KindHeartbeat
		return rec, true
	}
	return Snapshot{}, false
}
