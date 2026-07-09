// Command garminstatus collects the public Garmin Connect service status and
// turns it into the data behind a static status page.
//
// Subcommands:
//
//	status      fetch current status and print JSON (default)
//	snapshot    append a change-log record iff status changed / heartbeat
//	build       render site/data/status.json from the change-log
//	daemon      run the collect+publish loop (k8s)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/spf13/cobra"

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
	defaultInterval  = 5 * time.Minute
)

// fetchStatus is a seam so tests can drive the commands without a live fetch.
var fetchStatus = garminstatus.FetchStatus

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := rootCmd().ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "garminstatus",
		Short:         "Garmin Connect status collector",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: false,
		// Bare invocation prints the current status (back-compat).
		RunE: func(cmd *cobra.Command, _ []string) error { return doStatus(cmd.OutOrStdout()) },
	}
	root.AddCommand(statusCmd(), snapshotCmd(), buildCmd(), daemonCmd())
	return root
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Fetch current status and print JSON",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return doStatus(cmd.OutOrStdout()) },
	}
}

func snapshotCmd() *cobra.Command {
	var data string
	var heartbeat time.Duration
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Append a change-log record if the status changed",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return doSnapshot(cmd.OutOrStdout(), data, heartbeat) },
	}
	cmd.Flags().StringVar(&data, "data", defaultDataPath, "path to the change-log JSONL")
	cmd.Flags().DurationVar(&heartbeat, "heartbeat", defaultHeartbeat, "force a record after this long with no change")
	return cmd
}

func buildCmd() *cobra.Command {
	var data, out string
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Render the static-site data file from the change-log",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return doBuild(cmd.OutOrStdout(), data, out) },
	}
	cmd.Flags().StringVar(&data, "data", defaultDataPath, "path to the change-log JSONL")
	cmd.Flags().StringVar(&out, "out", defaultOutPath, "path to write the status JSON")
	return cmd
}

func daemonCmd() *cobra.Command {
	var (
		interval, heartbeat time.Duration
		repo, key, work     string
		addr                string
	)
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the collect + publish loop",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return doDaemon(cmd.Context(), interval, heartbeat, repo, key, work, addr)
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", defaultInterval, "collect/publish cycle interval")
	cmd.Flags().DurationVar(&heartbeat, "heartbeat", defaultHeartbeat, "force a record after this long with no change")
	cmd.Flags().StringVar(&repo, "repo", "", "git remote SSH URL (e.g. git@github.com:owner/repo.git)")
	cmd.Flags().StringVar(&key, "key", "", "path to the SSH private key (deploy key)")
	cmd.Flags().StringVar(&work, "work", "/work", "working directory for branch clones")
	cmd.Flags().StringVar(&addr, "http", ":8080", "listen address for /metrics, /live, /ready")
	_ = cmd.MarkFlagRequired("repo")
	return cmd
}

// doStatus fetches and prints the current status as JSON.
func doStatus(w io.Writer) error {
	status, err := fetchStatus()
	if err != nil {
		return err
	}
	enc, err := json.Marshal(status)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(w, string(enc))
	return nil
}

func doSnapshot(w io.Writer, data string, heartbeat time.Duration) error {
	status, err := fetchStatus()
	if err != nil {
		return err
	}
	printStatusSummary(w, status)

	last, ok, err := store.Last(data)
	if err != nil {
		return err
	}
	var lastPtr *store.Snapshot
	if ok {
		lastPtr = &last
	}

	rec, write := store.Decide(lastPtr, status, time.Now(), heartbeat)
	if !write {
		_, _ = fmt.Fprintln(w, "unchanged: no record written")
		return nil
	}
	if err := store.Append(data, rec); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "wrote %s record at %s\n", rec.Kind, rec.TS.Format(time.RFC3339))
	return nil
}

func doBuild(w io.Writer, data, out string) error {
	snaps, err := store.ReadAll(data)
	if err != nil {
		return err
	}
	status := rollup.Build(snaps)

	enc, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir(out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, append(enc, '\n'), 0o644); err != nil { //nolint:gosec // operator-controlled path
		return err
	}
	_, _ = fmt.Fprintf(w, "wrote %s (%d platforms, %d features, %d incidents)\n",
		out, len(status.Services.Platforms), len(status.Services.Features), len(status.Incidents))
	return nil
}

// doDaemon builds the publisher, serves observability endpoints, and runs the
// cycle loop until ctx is cancelled.
func doDaemon(ctx context.Context, interval, heartbeat time.Duration, repo, key, work, addr string) error {
	pub, err := publish.New(repo, key, work, heartbeat)
	if err != nil {
		return err
	}
	go func() {
		if err := ghttp.Serve(addr); err != nil {
			log.Println("http server:", err)
		}
	}()
	runLoop(ctx, interval, func() { logCycle(pub.Cycle()) })
	return nil
}

// runLoop runs cycle immediately, then every interval, until ctx is done.
func runLoop(ctx context.Context, interval time.Duration, cycle func()) {
	cycle()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return
		case <-ticker.C:
			cycle()
		}
	}
}

func logCycle(err error) {
	if err != nil {
		log.Println("cycle error:", err)
		return
	}
	log.Println("cycle ok")
}

// printStatusSummary logs what was gathered from Garmin: per-category counts
// and any services currently reported down.
func printStatusSummary(w io.Writer, status garminstatus.GarminStatus) {
	pUp, pDown, pDownNames := summarize(status.Platforms)
	fUp, fDown, fDownNames := summarize(status.Features)
	_, _ = fmt.Fprintf(w, "fetched Garmin status: platforms %d up / %d down, features %d up / %d down\n",
		pUp, pDown, fUp, fDown)
	down := append(pDownNames, fDownNames...)
	if len(down) == 0 {
		_, _ = fmt.Fprintln(w, "  all services operational")
		return
	}
	for _, name := range down {
		_, _ = fmt.Fprintf(w, "  DOWN: %s\n", name)
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

func dir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
