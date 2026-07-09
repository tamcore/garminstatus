# AGENTS.md

Guidance for humans and coding agents working in this repo.

## What this is

`garminstatus` is a Go tool that scrapes the **public** Garmin Connect status page and
turns it into a static uptime status page. It runs as a long-lived **daemon** that, each
cycle, fetches the status, appends to an append-only change-log, and publishes the rendered
site — pushing over an SSH deploy key to two dedicated branches. GitHub Pages serves the
site directly.

Live page: https://tamcore.github.io/garminstatus/

## Branch model

- **`master`** — code + static site source only. Protected; **CI must pass** to merge.
- **`data`** — the "database": `data/snapshots.jsonl`, an append-only change-log (a record
  only when a service's status changes, plus a periodic heartbeat).
- **`gh-pages`** — the published site (`index.html`, `app.js`, `style.css`,
  `data/status.json`); GitHub Pages source. The daemon writes it with normal fast-forward
  commits (never force-push — that orphans the Pages build).

## Layout

- `main.go` — CLI (cobra). Subcommands: `status` (default), `snapshot`, `build`, `daemon`.
- `pkg/garminstatus` — scrape + `GarminStatus`/`ServiceMap` types.
- `pkg/store` — change-log read/append/merge + change/heartbeat decision.
- `pkg/rollup` — time-weighted integration → per-service daily uptime + incidents.
- `pkg/publish` — go-git publisher (SSH, pinned GitHub host keys) for the data/gh-pages branches.
- `pkg/metrics`, `pkg/http`, `pkg/healthcheck` — Prometheus `/metrics`, `/live`, `/ready`.
- `site/` — static page, embedded into the binary via `//go:embed` (`site/embed.go`).

## Commands

```sh
go run . status              # fetch current status, print JSON (default)
go run . snapshot            # append a change-log record if status changed
go run . build               # render site/data/status.json from a local change-log
go run . daemon --repo git@github.com:tamcore/garminstatus.git --key <ssh-key>
```

## Development

- Test: `go test ./...` (TDD — write tests first; keep coverage high).
- Lint: golangci-lint **v2** (`.golangci.yaml`).
- Hooks: `pre-commit run --all-files` (file hygiene + go test/vet/fmt + golangci-lint).
- Conventions: cobra for CLI arg handling; small, cohesive files; immutable data patterns;
  conventional commit messages; keep the site self-contained (no external assets).

## Release

Tag `vX.Y.Z` → the release workflow builds the container image (goreleaser + ko →
`ghcr.io/tamcore/garminstatus`).

---

Local/private operational notes (deploy, infrastructure) live in **`AGENTS.md.local`**,
which is gitignored and not part of this public repo.
