package publish

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
	"github.com/tamcore/garminstatus/pkg/store"
)

func testSig() *object.Signature {
	return &object.Signature{Name: "seed", Email: "seed@example.com", When: time.Unix(0, 0).UTC()}
}

// seedRemote creates a bare repo with an empty `data` branch and returns its
// file:// URL.
func seedRemote(t *testing.T) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "remote.git")
	if _, err := git.PlainInit(bare, true); err != nil {
		t.Fatalf("init bare: %v", err)
	}
	url := "file://" + bare

	work := t.TempDir()
	r, err := git.PlainInit(work, false)
	if err != nil {
		t.Fatalf("init seed: %v", err)
	}
	// Make the first commit land on refs/heads/data (orphan branch).
	if err := r.Storer.SetReference(
		plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(dataBranch)),
	); err != nil {
		t.Fatalf("set HEAD: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(work, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, dataRelPath), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	w, _ := r.Worktree()
	if _, err := w.Add(dataRelPath); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit("seed", &git.CommitOptions{Author: testSig(), AllowEmptyCommits: true}); err != nil {
		t.Fatalf("seed commit: %v", err)
	}
	// seed gh-pages (orphan) so publishPages can clone it
	if err := r.Storer.SetReference(
		plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(pagesBranch)),
	); err != nil {
		t.Fatalf("set HEAD gh-pages: %v", err)
	}
	if err := os.WriteFile(filepath.Join(work, "index.html"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Add("index.html"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit("seed gh-pages", &git.CommitOptions{Author: testSig()}); err != nil {
		t.Fatalf("seed gh-pages commit: %v", err)
	}
	rem, err := r.CreateRemote(&config.RemoteConfig{Name: originRemote, URLs: []string{url}})
	if err != nil {
		t.Fatal(err)
	}
	if err := rem.Push(&git.PushOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec("refs/heads/data:refs/heads/data"),
			config.RefSpec("refs/heads/gh-pages:refs/heads/gh-pages"),
		},
	}); err != nil {
		t.Fatalf("seed push: %v", err)
	}
	return url
}

func up(name string) garminstatus.GarminStatus {
	return garminstatus.GarminStatus{
		Platforms: garminstatus.ServiceMap{name: {Status: garminstatus.Up}},
		Features:  garminstatus.ServiceMap{},
	}
}
func down(name string) garminstatus.GarminStatus {
	return garminstatus.GarminStatus{
		Platforms: garminstatus.ServiceMap{name: {Status: garminstatus.Down}},
		Features:  garminstatus.ServiceMap{},
	}
}

func dataRecords(t *testing.T, url string) []store.Snapshot {
	t.Helper()
	dir := t.TempDir()
	if _, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL: url, ReferenceName: plumbing.NewBranchReferenceName(dataBranch), SingleBranch: true,
	}); err != nil {
		t.Fatalf("clone data: %v", err)
	}
	snaps, err := store.ReadAll(filepath.Join(dir, dataRelPath))
	if err != nil {
		t.Fatal(err)
	}
	return snaps
}

func pagesHasStatus(t *testing.T, url string) {
	t.Helper()
	dir := t.TempDir()
	if _, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL: url, ReferenceName: plumbing.NewBranchReferenceName(pagesBranch), SingleBranch: true,
	}); err != nil {
		t.Fatalf("clone gh-pages: %v", err)
	}
	for _, f := range []string{"index.html", "app.js", "style.css", "data/status.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("gh-pages missing %s", f)
		}
	}
}

func newTestPublisher(
	url, work string,
	fetch func() (garminstatus.GarminStatus, error),
	now func() time.Time,
) *Publisher {
	return &Publisher{repoURL: url, workDir: work, heartbeat: 3 * time.Hour, fetch: fetch, now: now}
}

func TestCycleWritesDataAndPages(t *testing.T) {
	url := seedRemote(t)
	cur := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	p := newTestPublisher(url, t.TempDir(),
		func() (garminstatus.GarminStatus, error) { return up("A"), nil },
		func() time.Time { return cur },
	)

	if err := p.Cycle(); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if n := len(dataRecords(t, url)); n != 1 {
		t.Fatalf("expected 1 data record, got %d", n)
	}
	pagesHasStatus(t, url)
}

func TestCycleDedupAndChange(t *testing.T) {
	url := seedRemote(t)
	cur := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	status := up("A")
	p := newTestPublisher(url, t.TempDir(),
		func() (garminstatus.GarminStatus, error) { return status, nil },
		func() time.Time { return cur },
	)

	if err := p.Cycle(); err != nil { // record #1
		t.Fatal(err)
	}
	cur = cur.Add(time.Minute) // unchanged, within heartbeat -> no new record
	if err := p.Cycle(); err != nil {
		t.Fatal(err)
	}
	if n := len(dataRecords(t, url)); n != 1 {
		t.Fatalf("expected still 1 record after no-change cycle, got %d", n)
	}
	cur = cur.Add(time.Minute) // status changes -> record #2
	status = down("A")
	if err := p.Cycle(); err != nil {
		t.Fatal(err)
	}
	if n := len(dataRecords(t, url)); n != 2 {
		t.Fatalf("expected 2 records after change, got %d", n)
	}
}
