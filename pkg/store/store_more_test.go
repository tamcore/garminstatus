package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
)

func TestWriteAllRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "s.jsonl")
	t0 := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	in := []Snapshot{
		{TS: t0, Kind: KindChange, Platforms: up("A"), Features: garminstatus.ServiceMap{}},
		{TS: t0.Add(time.Hour), Kind: KindChange, Platforms: down(), Features: garminstatus.ServiceMap{}},
	}
	if err := WriteAll(path, in); err != nil {
		t.Fatalf("WriteAll: %v", err)
	}
	got, err := ReadAll(path)
	if err != nil || len(got) != 2 {
		t.Fatalf("ReadAll after WriteAll: n=%d err=%v", len(got), err)
	}
}

func TestSameStatus(t *testing.T) {
	a := Snapshot{Platforms: up("A"), Features: garminstatus.ServiceMap{}}
	b := Snapshot{Platforms: up("A"), Features: garminstatus.ServiceMap{}}
	if !SameStatus(a, b) {
		t.Error("identical projections should be equal")
	}
	if SameStatus(a, Snapshot{Platforms: down(), Features: garminstatus.ServiceMap{}}) {
		t.Error("different status should not be equal")
	}
	two := garminstatus.ServiceMap{"A": {Status: garminstatus.Up}, "B": {Status: garminstatus.Up}}
	if SameStatus(a, Snapshot{Platforms: two, Features: garminstatus.ServiceMap{}}) {
		t.Error("different cardinality should not be equal")
	}
}

func TestCollapse(t *testing.T) {
	t0 := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	snaps := []Snapshot{
		{TS: t0.Add(2 * time.Hour), Kind: KindChange, Platforms: up("A")},
		{TS: t0, Kind: KindChange, Platforms: up("A")},
		{TS: t0.Add(time.Hour), Kind: KindHeartbeat, Platforms: up("A")}, // unchanged -> dropped
		{TS: t0.Add(2 * time.Hour), Kind: KindChange, Platforms: down()}, // dup ts, keep last
	}
	out := Collapse(snaps)
	if len(out) != 2 {
		t.Fatalf("expected 2 collapsed, got %d: %+v", len(out), out)
	}
	if out[0].Platforms["A"].Status != garminstatus.Up || out[1].Platforms["A"].Status != garminstatus.Down {
		t.Errorf("unexpected collapse result: %+v", out)
	}
	if Collapse(nil) != nil {
		t.Error("Collapse(nil) should be nil")
	}
}

func TestAppendMkdirError(t *testing.T) {
	// A regular file in the parent position makes MkdirAll fail.
	base := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(base, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// base/child treats a file as a directory -> mkdir error
	if err := Append(filepath.Join(base, "child", "s.jsonl"), Snapshot{Kind: KindChange}); err == nil {
		t.Error("expected Append error when parent dir cannot be created")
	}
}

func TestReadAllParseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.jsonl")
	if err := os.WriteFile(path, []byte("{not json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAll(path); err == nil {
		t.Error("expected parse error for malformed JSONL")
	}
	if _, _, err := Last(path); err == nil {
		t.Error("Last should propagate parse error")
	}
}
