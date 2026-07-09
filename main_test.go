package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
)

func fakeStatus() garminstatus.GarminStatus {
	return garminstatus.GarminStatus{
		Platforms: garminstatus.ServiceMap{"Garmin Connect": {Status: garminstatus.Up}},
		Features:  garminstatus.ServiceMap{"Uploads": {Status: garminstatus.Down}},
	}
}

// withFetch swaps the fetchStatus seam for the duration of a test.
func withFetch(t *testing.T, fn func() (garminstatus.GarminStatus, error)) {
	t.Helper()
	orig := fetchStatus
	fetchStatus = fn
	t.Cleanup(func() { fetchStatus = orig })
}

// ---- pure helpers -------------------------------------------------------

func TestDir(t *testing.T) {
	if dir("a/b/c") != "a/b" || dir("x") != "." {
		t.Errorf("dir wrong: %q %q", dir("a/b/c"), dir("x"))
	}
}

func TestSummarize(t *testing.T) {
	m := garminstatus.ServiceMap{
		"U":  {Status: garminstatus.Up},
		"D2": {Status: garminstatus.Down},
		"D1": {Status: garminstatus.Down},
	}
	up, down, names := summarize(m)
	if up != 1 || down != 2 || names[0] != "D1" || names[1] != "D2" {
		t.Errorf("summarize wrong: %d %d %v", up, down, names)
	}
}

func TestPrintStatusSummary(t *testing.T) {
	var buf bytes.Buffer
	printStatusSummary(&buf, garminstatus.GarminStatus{
		Platforms: garminstatus.ServiceMap{"A": {Status: garminstatus.Up}}, Features: garminstatus.ServiceMap{},
	})
	printStatusSummary(&buf, fakeStatus())
	if !bytes.Contains(buf.Bytes(), []byte("all services operational")) ||
		!bytes.Contains(buf.Bytes(), []byte("DOWN: Uploads")) {
		t.Errorf("summary output missing lines:\n%s", buf.String())
	}
}

// ---- do* logic ----------------------------------------------------------

func TestDoStatus(t *testing.T) {
	withFetch(t, func() (garminstatus.GarminStatus, error) { return fakeStatus(), nil })
	var buf bytes.Buffer
	if err := doStatus(&buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"Garmin Connect"`)) {
		t.Errorf("status JSON missing service: %s", buf.String())
	}

	withFetch(t, func() (garminstatus.GarminStatus, error) { return garminstatus.GarminStatus{}, errors.New("net") })
	if err := doStatus(&buf); err == nil {
		t.Error("doStatus should propagate fetch error")
	}
}

func TestDoSnapshot(t *testing.T) {
	withFetch(t, func() (garminstatus.GarminStatus, error) { return fakeStatus(), nil })
	data := filepath.Join(t.TempDir(), "snap.jsonl")
	var buf bytes.Buffer

	if err := doSnapshot(&buf, data, time.Hour); err != nil { // first -> writes change
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("wrote change record")) {
		t.Errorf("expected write: %s", buf.String())
	}
	buf.Reset()
	if err := doSnapshot(&buf, data, time.Hour); err != nil { // unchanged, within heartbeat
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("unchanged")) {
		t.Errorf("expected unchanged: %s", buf.String())
	}

	withFetch(t, func() (garminstatus.GarminStatus, error) { return garminstatus.GarminStatus{}, errors.New("net") })
	if err := doSnapshot(&buf, data, time.Hour); err == nil {
		t.Error("doSnapshot should propagate fetch error")
	}
}

func TestDoBuild(t *testing.T) {
	data := filepath.Join(t.TempDir(), "snap.jsonl")
	line := `{"ts":"2026-01-10T00:00:00Z","kind":"change","platforms":{"A":{"status":"up"}},"features":{}}` + "\n"
	if err := os.WriteFile(data, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "sub", "status.json")
	var buf bytes.Buffer
	if err := doBuild(&buf, data, out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("out not written: %v", err)
	}
}

func TestDoBuildErrors(t *testing.T) {
	// unreadable/malformed change-log -> ReadAll error
	bad := filepath.Join(t.TempDir(), "bad.jsonl")
	_ = os.WriteFile(bad, []byte("{not json}\n"), 0o644)
	var buf bytes.Buffer
	if err := doBuild(&buf, bad, filepath.Join(t.TempDir(), "o.json")); err == nil {
		t.Error("doBuild should error on malformed change-log")
	}
	// out under a regular file -> MkdirAll error
	base := filepath.Join(t.TempDir(), "afile")
	_ = os.WriteFile(base, []byte("x"), 0o644)
	data := filepath.Join(t.TempDir(), "s.jsonl")
	_ = os.WriteFile(data, []byte(""), 0o644)
	if err := doBuild(&buf, data, filepath.Join(base, "child", "o.json")); err == nil {
		t.Error("doBuild should error when out dir cannot be created")
	}
	// out is an existing directory -> WriteFile error
	if err := doBuild(&buf, data, t.TempDir()); err == nil {
		t.Error("doBuild should error when out is a directory")
	}
}

func TestDoSnapshotErrors(t *testing.T) {
	withFetch(t, func() (garminstatus.GarminStatus, error) { return fakeStatus(), nil })
	var buf bytes.Buffer
	// malformed change-log -> store.Last error
	bad := filepath.Join(t.TempDir(), "bad.jsonl")
	_ = os.WriteFile(bad, []byte("{not json}\n"), 0o644)
	if err := doSnapshot(&buf, bad, time.Hour); err == nil {
		t.Error("doSnapshot should error on malformed change-log")
	}
	// data path whose parent cannot be created -> store.Append error
	base := filepath.Join(t.TempDir(), "afile")
	_ = os.WriteFile(base, []byte("x"), 0o644)
	if err := doSnapshot(&buf, filepath.Join(base, "child", "s.jsonl"), time.Hour); err == nil {
		t.Error("doSnapshot should error when data dir cannot be created")
	}
}

// ---- daemon loop --------------------------------------------------------

func TestRunLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	done := make(chan struct{})
	go func() {
		runLoop(ctx, time.Millisecond, func() {
			n++
			if n >= 3 {
				cancel()
			}
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runLoop did not return after cancel")
	}
	if n < 3 {
		t.Errorf("expected >=3 cycles, got %d", n)
	}
	logCycle(nil)
	logCycle(errors.New("boom"))
}

func TestDoDaemonBadKey(t *testing.T) {
	err := doDaemon(context.Background(), time.Minute, time.Hour,
		"git@github.com:o/r.git", "/does/not/exist", t.TempDir(), "127.0.0.1:0")
	if err == nil {
		t.Error("doDaemon should error on unreadable key")
	}
}

func TestDoDaemonRuns(t *testing.T) {
	url := seedBareRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- doDaemon(ctx, 10*time.Millisecond, time.Hour, url, "", t.TempDir(), "127.0.0.1:0") }()
	time.Sleep(300 * time.Millisecond) // let the first cycle run
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("doDaemon returned error: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("doDaemon did not return after cancel")
	}
}

func TestDoDaemonServeError(t *testing.T) {
	url := seedBareRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	// invalid listen address -> Serve errors in its goroutine (logged, non-fatal)
	go func() { done <- doDaemon(ctx, 10*time.Millisecond, time.Hour, url, "", t.TempDir(), "bad-addr!!!") }()
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("doDaemon should still return nil (serve error is logged): %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("doDaemon did not return")
	}
}

// ---- cobra wiring -------------------------------------------------------

func execRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	c := rootCmd()
	var buf bytes.Buffer
	c.SetOut(&buf)
	c.SetErr(&buf)
	c.SetArgs(args)
	err := c.Execute()
	return buf.String(), err
}

func TestCLIWiring(t *testing.T) {
	withFetch(t, func() (garminstatus.GarminStatus, error) { return fakeStatus(), nil })

	if out, err := execRoot(t); err != nil || !bytes.Contains([]byte(out), []byte("Garmin Connect")) {
		t.Errorf("bare root: out=%q err=%v", out, err)
	}
	if _, err := execRoot(t, "status"); err != nil {
		t.Errorf("status: %v", err)
	}
	data := filepath.Join(t.TempDir(), "s.jsonl")
	if _, err := execRoot(t, "snapshot", "--data", data, "--heartbeat", "1s"); err != nil {
		t.Errorf("snapshot: %v", err)
	}
	out := filepath.Join(t.TempDir(), "status.json")
	if _, err := execRoot(t, "build", "--data", data, "--out", out); err != nil {
		t.Errorf("build: %v", err)
	}
	if o, err := execRoot(t, "--version"); err != nil || !bytes.Contains([]byte(o), []byte(Version)) {
		t.Errorf("version: out=%q err=%v", o, err)
	}
	if _, err := execRoot(t, "daemon"); err == nil {
		t.Error("daemon without --repo should error")
	}
}

func TestCLIDaemon(t *testing.T) {
	url := seedBareRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	c := rootCmd()
	c.SetArgs([]string{"daemon", "--repo", url, "--http", "127.0.0.1:0", "--interval", "10ms", "--work", t.TempDir()})
	done := make(chan error, 1)
	go func() { done <- c.ExecuteContext(ctx) }()
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("daemon command: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("daemon command did not return after cancel")
	}
}

// ---- test git seed ------------------------------------------------------

func seedBareRepo(t *testing.T) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "remote.git")
	if _, err := git.PlainInit(bare, true); err != nil {
		t.Fatal(err)
	}
	url := "file://" + bare
	work := t.TempDir()
	r, err := git.PlainInit(work, false)
	if err != nil {
		t.Fatal(err)
	}
	sig := &object.Signature{Name: "seed", Email: "seed@x", When: time.Unix(0, 0).UTC()}
	setHead := func(branch string) {
		ref := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(branch))
		if err := r.Storer.SetReference(ref); err != nil {
			t.Fatal(err)
		}
	}
	setHead("data")
	_ = os.MkdirAll(filepath.Join(work, "data"), 0o755)
	_ = os.WriteFile(filepath.Join(work, "data/snapshots.jsonl"), []byte(""), 0o644)
	w, _ := r.Worktree()
	_, _ = w.Add("data/snapshots.jsonl")
	if _, err := w.Commit("seed data", &git.CommitOptions{Author: sig, AllowEmptyCommits: true}); err != nil {
		t.Fatal(err)
	}
	setHead("gh-pages")
	_ = os.WriteFile(filepath.Join(work, "index.html"), []byte("seed"), 0o644)
	_, _ = w.Add("index.html")
	if _, err := w.Commit("seed gh-pages", &git.CommitOptions{Author: sig}); err != nil {
		t.Fatal(err)
	}
	rem, err := r.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{url}})
	if err != nil {
		t.Fatal(err)
	}
	if err := rem.Push(&git.PushOptions{RefSpecs: []config.RefSpec{
		"refs/heads/data:refs/heads/data", "refs/heads/gh-pages:refs/heads/gh-pages",
	}}); err != nil {
		t.Fatal(err)
	}
	return url
}
