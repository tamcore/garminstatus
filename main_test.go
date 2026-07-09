package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
)

func TestDir(t *testing.T) {
	if dir("a/b/c") != "a/b" {
		t.Errorf("dir(a/b/c) = %q", dir("a/b/c"))
	}
	if dir("x") != "." {
		t.Errorf("dir(x) = %q", dir("x"))
	}
}

func TestSummarize(t *testing.T) {
	m := garminstatus.ServiceMap{
		"Up1":   {Status: garminstatus.Up},
		"Down2": {Status: garminstatus.Down},
		"Down1": {Status: garminstatus.Down},
	}
	up, down, names := summarize(m)
	if up != 1 || down != 2 {
		t.Errorf("counts up=%d down=%d", up, down)
	}
	if len(names) != 2 || names[0] != "Down1" || names[1] != "Down2" {
		t.Errorf("down names not sorted: %v", names)
	}
}

func TestPrintStatusSummary(t *testing.T) {
	t.Helper()
	printStatusSummary(garminstatus.GarminStatus{
		Platforms: garminstatus.ServiceMap{"A": {Status: garminstatus.Up}},
		Features:  garminstatus.ServiceMap{},
	})
	printStatusSummary(garminstatus.GarminStatus{
		Platforms: garminstatus.ServiceMap{"A": {Status: garminstatus.Down}},
		Features:  garminstatus.ServiceMap{"B": {Status: garminstatus.Up}},
	})
}

// Best-effort: these hit the live Garmin page. They cover the command paths
// when the network is available and simply tolerate a fetch error otherwise.
func TestRunStatusSnapshotUsage(t *testing.T) {
	usage() // writes help to stderr
	if err := runStatus(); err != nil {
		t.Logf("runStatus (network) errored, tolerated: %v", err)
	}
	dataPath := filepath.Join(t.TempDir(), "snap.jsonl")
	if err := runSnapshot([]string{"--data", dataPath, "--heartbeat", "1s"}); err != nil {
		t.Logf("runSnapshot (network) errored, tolerated: %v", err)
	}
}

func TestRunBuild(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "snap.jsonl")
	line := `{"ts":"2026-01-10T00:00:00Z","kind":"change","platforms":{"A":{"status":"up"}},"features":{}}` + "\n"
	if err := os.WriteFile(dataPath, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(t.TempDir(), "out", "status.json")
	if err := runBuild([]string{"--data", dataPath, "--out", outPath}); err != nil {
		t.Fatalf("runBuild: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("status.json not written: %v", err)
	}
}
