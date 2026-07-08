# garminstatus

An **unofficial** status page for Garmin Connect, with 180 days of uptime history —
generated entirely by GitHub Actions and served from GitHub Pages. No servers, no
database.

> Live page: **https://tamcore.github.io/garminstatus/**
>
> Not affiliated with Garmin. Status is scraped from Garmin's public status page.

## How it works

```
                cron (*/5)                 on change            push to site/**
Garmin status ────────────▶ snapshot ─────────────▶ build ───────────────────▶ GitHub Pages
 (public page)   go run .   change-log    go run .   status.json   commit        (static site)
                            data/*.jsonl             site/data/*
```

1. **collect** workflow runs every 5 minutes (GitHub's minimum). It scrapes the public
   Garmin Connect status page and appends a record to the change-log **only when a
   service's status actually changes** — plus a heartbeat every few hours so freshness
   still advances. Unchanged polls produce no commit, keeping git history quiet.
2. When the change-log changes, it re-renders `site/data/status.json`.
3. The **pages** workflow publishes `site/` to GitHub Pages whenever it changes.

Uptime is reconstructed by integrating the state **between** recorded transitions, so the
change-log stays tiny (one line per real change) while the page still shows a full
per-service daily timeline.

## The binary

Everything runs via `go run .` — there is no release artifact.

```sh
go run . status              # fetch current status, print JSON (default)
go run . snapshot            # append a change-log record if status changed
go run . build               # render site/data/status.json from the change-log
```

Flags:

| command    | flag          | default                  | meaning                                        |
|------------|---------------|--------------------------|------------------------------------------------|
| `snapshot` | `--data`      | `data/snapshots.jsonl`   | change-log path                                |
| `snapshot` | `--heartbeat` | `3h`                     | force a record after this long with no change  |
| `build`    | `--data`      | `data/snapshots.jsonl`   | change-log path                                |
| `build`    | `--out`       | `site/data/status.json`  | rendered output path                           |

## Data formats

**`data/snapshots.jsonl`** — append-only change-log, one JSON object per line:

```json
{"ts":"2026-07-08T12:00:00Z","kind":"change","platforms":{"Garmin Connect":{"status":"up"}},"features":{}}
```

- `kind` is `change` (a status transition) or `heartbeat` (freshness only).
- `platforms` / `features` map each service name to `{status: "up"|"down", status_reason?: [...]}`.

**`site/data/status.json`** — pre-aggregated artifact the page reads. Absolute-dated daily
buckets, so it changes only when the underlying data changes:

```json
{
  "generated": "2026-07-08T12:00:00Z",
  "dataThrough": "2026-07-08T12:00:00Z",
  "services": {
    "platforms": [
      {"name":"Garmin Connect","current":"up","days":[{"date":"2026-04-01","upFrac":0.993,"worst":"down"}]}
    ],
    "features": []
  },
  "incidents": [
    {"service":"Garmin Connect","start":"...","end":"...","reasons":[]}
  ]
}
```

The page slices the last 180 days relative to the viewer's clock and colors each day by its
uptime band. Very short blips are debounced out of the incident list.

## Local development

```sh
go test ./...                       # unit tests
go run . snapshot && go run . build # produce a local status.json
cd site && python3 -m http.server   # open http://localhost:8000
```

## License

See repository metadata.
