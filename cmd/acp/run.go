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
	server := fs.String("server", "", "control plane URL (default $ACP_SERVER or "+defaultServer+")")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *agent == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	url := firstNonEmpty(*server, getenv("ACP_SERVER"), defaultServer)

	// No client timeout: a turn runs as long as the agent works.
	c := httpapi.NewClient(url, getenv("ACP_API_TOKEN"), &http.Client{})
	resp, err := c.RunTurn(ctx, httpapi.TurnRequest{Agent: *agent, SessionID: *session, Prompt: strings.Join(fs.Args(), " ")})

	var apiErr *httpapi.APIError
	if errors.As(err, &apiErr) {
		printSession(stderr, apiErr.SessionID, apiErr.WorkspacePath)
		fmt.Fprintln(stderr, "acp:", apiErr)
		if apiErr.Status >= 400 && apiErr.Status < 500 {
			return 2
		}
		return 1
	}
	if err != nil {
		fmt.Fprintln(stderr, "acp:", err)
		return 1
	}
	printSession(stderr, resp.SessionID, resp.WorkspacePath)
	fmt.Fprint(stdout, resp.Output)
	return 0
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
