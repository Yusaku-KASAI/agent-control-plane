package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

const usage = `usage:
  acp serve [--config <path>] [--listen <addr>]
  acp run --agent <name> [--session <id>] [--inject] [--server <url>] <prompt>
  acp stop --agent <name> --session <id> [--server <url>]
  acp slack-auth --agent <name> [--config <path>] [--code <code> [--redirect-uri <url>]]
  acp workspace-state
  acp cleanup-check

serve, run and stop read the API token from ACP_API_TOKEN. run and stop read the server URL from
ACP_SERVER when --server is not given. workspace-state prints, as JSON, what the repository in the
working directory holds for the session cleanup; the control plane runs it in a container.
cleanup-check prints what keeps the session in the working directory from the cleanup; agents run it
in their containers.`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(dispatch(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func dispatch(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "serve":
			return serve(ctx, args[1:], getenv, stderr)
		case "run":
			return runTurn(ctx, args[1:], getenv, stdout, stderr)
		case "stop":
			return stop(ctx, args[1:], getenv, stderr)
		case "slack-auth":
			return slackAuth(ctx, args[1:], stdout, stderr)
		case "workspace-state":
			return workspaceState(ctx, stdout, stderr)
		case "cleanup-check":
			return cleanupCheck(ctx, getenv, stdout, stderr)
		}
	}
	fmt.Fprintln(stderr, usage)
	return 2
}
