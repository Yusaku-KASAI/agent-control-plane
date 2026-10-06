package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/config"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/docker"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/filesession"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

const defaultPruneDays = 14

// prune removes the workspaces, records and credentials of sessions idle for too long. It must
// run where the sessions directory is, such as in the control plane's container.
func prune(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "acp.yaml", "config file path")
	days := fs.Int("older-than-days", defaultPruneDays, "remove sessions idle for more than this many days")
	dryRun := fs.Bool("dry-run", false, "only list the sessions that would be removed")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *days <= 0 {
		fmt.Fprintln(stderr, "acp: --older-than-days must be positive")
		return 2
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, "acp:", err)
		return 1
	}
	uc := usecase.PruneSessions{
		Store:       filesession.NewStore(cfg.SessionsRoot),
		Credentials: docker.CredentialStore{Root: cfg.Credentials.Dir},
		Now:         time.Now,
	}
	pruned, err := uc.Run(ctx, time.Duration(*days)*24*time.Hour, *dryRun)
	verb := "removed"
	if *dryRun {
		verb = "would remove"
	}
	for _, p := range pruned {
		fmt.Fprintf(stdout, "%s %s (last active %s)\n", verb, p.ID, p.LastActive.Format(time.DateOnly))
	}
	if err != nil {
		fmt.Fprintln(stderr, "acp:", err)
		return 1
	}
	return 0
}
