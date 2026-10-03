package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/config"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

const slackAuthSteps = `To recover the Slack tokens of agent %q:

  1. In the Slack app settings, open Manage Distribution and open the Sharable URL in a browser.
  2. Approve. The browser is sent to your Redirect URL, which need not load; copy the code
     parameter from the address bar.
  3. Run, within ten minutes:
       acp slack-auth --agent %s --code <code> --redirect-uri <the redirect_uri in the Sharable URL, if any>
  4. Restart the control plane so that it loads the new tokens.
`

// slackAuth recovers the rotating Slack tokens of an agent from an OAuth authorization code.
func slackAuth(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("slack-auth", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "acp.yaml", "config file path")
	agent := fs.String("agent", "", "agent whose Slack app uses token rotation")
	code := fs.String("code", "", "OAuth authorization code; omit to print the steps")
	redirectURI := fs.String("redirect-uri", "", "redirect_uri used in the authorization, if any")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *agent == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if *code == "" {
		fmt.Fprintf(stdout, slackAuthSteps, *agent, *agent)
		return 0
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, "acp:", err)
		return 1
	}
	app, ok := cfg.SlackApps[domain.AgentName(*agent)]
	if !ok || app.Rotation == nil {
		fmt.Fprintf(stderr, "acp: agent %q has no Slack app with token rotation\n", *agent)
		return 2
	}
	_, rotating, err := slackTokenSources(config.Config{
		SlackApps: map[domain.AgentName]config.SlackApp{domain.AgentName(*agent): app},
		StateDir:  cfg.StateDir,
	})
	if err != nil {
		fmt.Fprintln(stderr, "acp:", err)
		return 1
	}
	src := rotating[domain.AgentName(*agent)]
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := src.RedeemCode(ctx, *code, *redirectURI); err != nil {
		fmt.Fprintln(stderr, "acp:", err)
		return 1
	}
	fmt.Fprintln(stdout, "Saved new Slack tokens. Restart the control plane so that it loads them.")
	return 0
}
