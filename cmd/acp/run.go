package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/httpapi"
)

const defaultServer = "http://127.0.0.1:8787"

func runTurn(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	agent := fs.String("agent", "", "agent name defined in the server config")
	session := fs.String("session", "", "session ID to continue; omit to start a new session")
	inject := fs.Bool("inject", false, "send into the running turn instead of queueing behind it")
	server := serverFlag(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *agent == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}

	resp, err := apiClient(*server, getenv).RunTurn(ctx, httpapi.TurnRequest{
		Agent: *agent, SessionID: *session, Prompt: strings.Join(fs.Args(), " "), Inject: *inject,
	})
	if err != nil {
		return reportError(stderr, err)
	}
	printSession(stderr, resp.SessionID, resp.WorkspacePath)
	if resp.Interrupted {
		fmt.Fprintln(stderr, "acp: the turn was interrupted")
	}
	fmt.Fprint(stdout, resp.Output)
	return 0
}

func interrupt(ctx context.Context, args []string, getenv func(string) string, stderr io.Writer) int {
	fs := flag.NewFlagSet("interrupt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	agent := fs.String("agent", "", "agent name defined in the server config")
	session := fs.String("session", "", "session ID whose running turn to stop")
	server := serverFlag(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *agent == "" || *session == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if err := apiClient(*server, getenv).Interrupt(ctx, *session, httpapi.InterruptRequest{Agent: *agent}); err != nil {
		return reportError(stderr, err)
	}
	return 0
}

func serverFlag(fs *flag.FlagSet) *string {
	return fs.String("server", "", "control plane URL (default $ACP_SERVER or "+defaultServer+")")
}

func apiClient(server string, getenv func(string) string) *httpapi.Client {
	url := firstNonEmpty(server, getenv("ACP_SERVER"), defaultServer)
	// No client timeout: a turn runs as long as the agent works.
	return httpapi.NewClient(url, getenv("ACP_API_TOKEN"), &http.Client{})
}

func reportError(stderr io.Writer, err error) int {
	var apiErr *httpapi.APIError
	if !errors.As(err, &apiErr) {
		fmt.Fprintln(stderr, "acp:", err)
		return 1
	}
	printSession(stderr, apiErr.SessionID, apiErr.WorkspacePath)
	fmt.Fprintln(stderr, "acp:", apiErr)
	if apiErr.Status >= 400 && apiErr.Status < 500 {
		return 2
	}
	return 1
}

func printSession(w io.Writer, id, workspace string) {
	if id == "" {
		return
	}
	fmt.Fprintln(w, "acp: session:", id)
	fmt.Fprintln(w, "acp: workspace:", workspace)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
