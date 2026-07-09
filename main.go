// Command garminstatus collects the public Garmin Connect service status and
// turns it into the data behind a static status page.
//
// Subcommands:
//
//	status            fetch current status and print JSON (default)
//	snapshot          append a change-log record iff status changed / heartbeat
//	build             render site/data/status.json from the change-log
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
	ghttp "github.com/tamcore/garminstatus/pkg/http"
	"github.com/tamcore/garminstatus/pkg/publish"
	"github.com/tamcore/garminstatus/pkg/rollup"
	"github.com/tamcore/garminstatus/pkg/store"
)

// Version is set at release time via -ldflags.
var Version = "dev"

const (
	defaultDataPath  = "data/snapshots.jsonl"
	defaultOutPath   = "site/data/status.json"
	defaultHeartbeat = 3 * time.Hour
)

func main() {
	args := os.Args[1:]
	cmd := "status"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	var err error
	switch cmd {
	case "status":
		err = runStatus()
	case "snapshot":
		err = runSnapshot(args)
	case "build":
		err = runBuild(args)
	case "daemon":
		err = runDaemon(args)
	case "version", "-v", "--version":
		fmt.Println(Version)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `garminstatus — Garmin Connect status collector

Usage:
  garminstatus status                 fetch current status, print JSON (default)
  garminstatus snapshot [flags]       append a change-log record if status changed
  garminstatus build [flags]          render the static-site data file
  garminstatus daemon [flags]         run the collect+publish loop (k8s)

Run "garminstatus <command> -h" for flags.
`)
}

// runStatus fetches and prints the current status as JSON (back-compat).
func runStatus() error {
	status, err := garminstatus.FetchStatus()
	if err != nil {
		return err
	}
	enc, err := json.Marshal(status)
	if err != nil {
		return err
	}
	fmt.Println(string(enc))
	return nil
}

func runSnapshot(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
	data := fs.String("data", defaultDataPath, "path to the change-log JSONL")
	heartbeat := fs.Duration("heartbeat", defaultHeartbeat, "force a record after this long with no change (0 to disable)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	status, err := garminstatus.FetchStatus()
	if err != nil {
		return err
	}
	printStatusSummary(status)

	last, ok, err := store.Last(*data)
	if err != nil {
		return err
	}
	var lastPtr *store.Snapshot
	if ok {
		lastPtr = &last
	}

	rec, write := store.Decide(lastPtr, status, time.Now(), *heartbeat)
	if !write {
		fmt.Println("unchanged: no record written")
		return nil
	}
	if err := store.Append(*data, rec); err != nil {
		return err
	}
	fmt.Printf("wrote %s record at %s\n", rec.Kind, rec.TS.Format(time.RFC3339))
	return nil
}

func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	data := fs.String("data", defaultDataPath, "path to the change-log JSONL")
	out := fs.String("out", defaultOutPath, "path to write the status JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	snaps, err := store.ReadAll(*data)
	if err != nil {
		return err
	}
	status := rollup.Build(snaps)

	enc, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir(*out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(*out, append(enc, '\n'), 0o644); err != nil { //nolint:gosec // operator-controlled path
		return err
	}
	fmt.Printf("wrote %s (%d platforms, %d features, %d incidents)\n",
		*out, len(status.Services.Platforms), len(status.Services.Features), len(status.Incidents))
	return nil
}

// printStatusSummary logs what was gathered from Garmin: per-category counts
// and any services currently reported down.
func printStatusSummary(status garminstatus.GarminStatus) {
	pUp, pDown, pDownNames := summarize(status.Platforms)
	fUp, fDown, fDownNames := summarize(status.Features)
	fmt.Printf("fetched Garmin status: platforms %d up / %d down, features %d up / %d down\n",
		pUp, pDown, fUp, fDown)
	down := append(pDownNames, fDownNames...)
	if len(down) == 0 {
		fmt.Println("  all services operational")
		return
	}
	for _, name := range down {
		fmt.Printf("  DOWN: %s\n", name)
	}
}

// summarize counts up/down services in a map and returns the down service names
// (sorted).
func summarize(m garminstatus.ServiceMap) (up, down int, downNames []string) {
	for name, info := range m {
		if info.Status == garminstatus.Up {
			up++
			continue
		}
		down++
		downNames = append(downNames, name)
	}
	sort.Strings(downNames)
	return up, down, downNames
}

func runDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	interval := fs.Duration("interval", 5*time.Minute, "collect/publish cycle interval")
	heartbeat := fs.Duration("heartbeat", defaultHeartbeat, "force a data record after this long with no change")
	repo := fs.String("repo", "", "git remote SSH URL (e.g. git@github.com:owner/repo.git)")
	key := fs.String("key", "", "path to the SSH private key (deploy key)")
	work := fs.String("work", "/work", "working directory for branch clones")
	addr := fs.String("http", ":8080", "listen address for /metrics, /live, /ready")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repo == "" {
		return fmt.Errorf("--repo is required")
	}

	pub, err := publish.New(*repo, *key, *work, *heartbeat)
	if err != nil {
		return err
	}

	// Observability server runs for the lifetime of the process.
	go func() {
		if err := ghttp.Serve(*addr); err != nil {
			log.Fatal("http server:", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cycle := func() {
		if err := pub.Cycle(); err != nil {
			log.Println("cycle error:", err)
			return
		}
		log.Println("cycle ok")
	}

	cycle() // run immediately, then on the interval
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return nil
		case <-ticker.C:
			cycle()
		}
	}
}

func dir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
