// Package publish is the daemon's engine: one Cycle fetches Garmin status,
// appends to the change-log on the `data` branch (only when it changed or a
// heartbeat is due), rebuilds the status JSON, and force-pushes the rendered
// site as a single rolling commit to `gh-pages`. All git work uses go-git over
// SSH — no git binary required.
package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
	"github.com/tamcore/garminstatus/pkg/metrics"
	"github.com/tamcore/garminstatus/pkg/rollup"
	"github.com/tamcore/garminstatus/pkg/store"
	"github.com/tamcore/garminstatus/site"
)

const (
	dataBranch   = "data"
	pagesBranch  = "gh-pages"
	dataRelPath  = "data/snapshots.jsonl"
	originRemote = "origin"
)

// Publisher runs collect/publish cycles against a repo's data and gh-pages
// branches.
type Publisher struct {
	repoURL   string
	auth      transport.AuthMethod
	workDir   string
	heartbeat time.Duration

	// injectable for tests
	fetch func() (garminstatus.GarminStatus, error)
	now   func() time.Time
}

// New builds a Publisher. keyPath enables SSH auth (empty → no auth, e.g. for
// file:// remotes in tests).
func New(repoURL, keyPath, workDir string, heartbeat time.Duration) (*Publisher, error) {
	var auth transport.AuthMethod
	if keyPath != "" {
		a, err := sshAuth(keyPath)
		if err != nil {
			return nil, err
		}
		auth = a
	}
	return &Publisher{
		repoURL:   repoURL,
		auth:      auth,
		workDir:   workDir,
		heartbeat: heartbeat,
		fetch:     garminstatus.FetchStatus,
		now:       time.Now,
	}, nil
}

func (p *Publisher) sig() *object.Signature {
	return &object.Signature{
		Name:  "garminstatus",
		Email: "garminstatus@users.noreply.github.com",
		When:  p.now(),
	}
}

// Cycle runs a single collect + publish iteration.
func (p *Publisher) Cycle() error {
	status, err := p.fetch()
	metrics.RecordFetch(err == nil)
	if err != nil {
		return fmt.Errorf("fetch status: %w", err)
	}
	metrics.UpdateStatus(status)

	snaps, err := p.syncData(status)
	if err != nil {
		return err
	}
	if err := p.publishPages(snaps); err != nil {
		return err
	}
	metrics.RecordCycle()
	return nil
}

// syncData pulls the data branch, appends a record if warranted, pushes, and
// returns the full change-log for rendering.
func (p *Publisher) syncData(status garminstatus.GarminStatus) ([]store.Snapshot, error) {
	dir := filepath.Join(p.workDir, "data")
	repo, err := p.openOrClone(dir, dataBranch)
	if err != nil {
		metrics.RecordSync(dataBranch, "pull", err)
		return nil, fmt.Errorf("open data branch: %w", err)
	}
	if err := p.pull(repo, dataBranch); err != nil {
		metrics.RecordSync(dataBranch, "pull", err)
		return nil, fmt.Errorf("pull data branch: %w", err)
	}
	metrics.RecordSync(dataBranch, "pull", nil)

	path := filepath.Join(dir, dataRelPath)
	snaps, err := store.ReadAll(path)
	if err != nil {
		return nil, err
	}
	var last *store.Snapshot
	if n := len(snaps); n > 0 {
		last = &snaps[n-1]
	}
	rec, write := store.Decide(last, status, p.now(), p.heartbeat)
	if !write {
		return snaps, nil
	}
	if err := store.Append(path, rec); err != nil {
		return nil, err
	}
	snaps = append(snaps, rec)

	if err := p.commitAndPush(repo, dataBranch, dataRelPath,
		fmt.Sprintf("chore(data): %s snapshot", rec.Kind)); err != nil {
		metrics.RecordSync(dataBranch, "push", err)
		return nil, fmt.Errorf("push data branch: %w", err)
	}
	metrics.RecordSync(dataBranch, "push", nil)
	return snaps, nil
}

// publishPages renders status.json (generated=now) plus the embedded site
// assets into a fresh single-commit tree and force-pushes it to gh-pages.
func (p *Publisher) publishPages(snaps []store.Snapshot) error {
	st := rollup.Build(snaps)
	st.Generated = p.now().UTC() // "last checked" — always fresh
	body, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Join(p.workDir, "pages")
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		return err
	}
	for _, name := range site.Files {
		data, err := site.Assets.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil { //nolint:gosec // static assets
			return err
		}
	}
	statusPath := filepath.Join(dir, "data", "status.json")
	if err := os.WriteFile(statusPath, append(body, '\n'), 0o644); err != nil { //nolint:gosec // generated
		return err
	}

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		return err
	}
	w, err := repo.Worktree()
	if err != nil {
		return err
	}
	if err := w.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return err
	}
	if _, err := w.Commit("publish status page", &git.CommitOptions{Author: p.sig()}); err != nil {
		return err
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: originRemote, URLs: []string{p.repoURL}}); err != nil {
		return err
	}
	head, err := repo.Head()
	if err != nil {
		return err
	}
	rs := config.RefSpec(fmt.Sprintf("+%s:refs/heads/%s", head.Name().String(), pagesBranch))
	err = repo.Push(&git.PushOptions{
		RemoteName: originRemote,
		RefSpecs:   []config.RefSpec{rs},
		Force:      true,
		Auth:       p.auth,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		metrics.RecordSync(pagesBranch, "push", err)
		return fmt.Errorf("push gh-pages: %w", err)
	}
	metrics.RecordSync(pagesBranch, "push", nil)
	return nil
}

func (p *Publisher) openOrClone(dir, branch string) (*git.Repository, error) {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return git.PlainOpen(dir)
	}
	return git.PlainClone(dir, false, &git.CloneOptions{
		URL:           p.repoURL,
		Auth:          p.auth,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
	})
}

func (p *Publisher) pull(repo *git.Repository, branch string) error {
	err := repo.Fetch(&git.FetchOptions{
		Auth: p.auth,
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branch, branch)),
		},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return err
	}
	ref, err := repo.Reference(plumbing.NewRemoteReferenceName(originRemote, branch), true)
	if err != nil {
		return err
	}
	w, err := repo.Worktree()
	if err != nil {
		return err
	}
	return w.Reset(&git.ResetOptions{Mode: git.HardReset, Commit: ref.Hash()})
}

func (p *Publisher) commitAndPush(repo *git.Repository, branch, relPath, msg string) error {
	w, err := repo.Worktree()
	if err != nil {
		return err
	}
	if _, err := w.Add(relPath); err != nil {
		return err
	}
	if _, err := w.Commit(msg, &git.CommitOptions{Author: p.sig()}); err != nil {
		return err
	}
	err = repo.Push(&git.PushOptions{
		Auth: p.auth,
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch)),
		},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return err
	}
	return nil
}
