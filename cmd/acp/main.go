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

All read the API token from ACP_API_TOKEN. run and stop read the server URL from
ACP_SERVER when --server is not given.`

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
		}
	}
	fmt.Fprintln(stderr, usage)
	return 2
}
