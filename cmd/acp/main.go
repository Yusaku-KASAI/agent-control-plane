package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/moby/moby/client"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/config"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/docker"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/filesession"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/githubapp"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/gitworkspace"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

const usage = `usage: acp run --agent <name> [--session <id>] [--config <path>] <prompt>`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "run" {
		fmt.Fprintln(stderr, usage)
		return 2
	}

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	agentName := fs.String("agent", "", "agent name defined in the config")
	sessionArg := fs.String("session", "", "session ID to continue; omit to start a new session")
	configPath := fs.String("config", "acp.yaml", "config file path")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *agentName == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	var sessionID domain.SessionID
	if *sessionArg != "" {
		id, err := domain.ParseSessionID(*sessionArg)
		if err != nil {
			fmt.Fprintln(stderr, "acp:", err)
			return 2
		}
		sessionID = id
	}

	out, err := runAgent(ctx, *configPath, usecase.RunAgentInput{
		AgentName: domain.AgentName(*agentName),
		SessionID: sessionID,
		Prompt:    strings.Join(fs.Args(), " "),
	})
	if out.Session.ID != "" {
		fmt.Fprintln(stderr, "acp: session:", out.Session.ID)
		fmt.Fprintln(stderr, "acp: workspace:", out.Session.Workspace.Path)
	}
	if err != nil {
		fmt.Fprintln(stderr, "acp:", err)
		if isUsageError(err) {
			return 2
		}
		return 1
	}
	fmt.Fprint(stdout, out.Output)
	return 0
}

func isUsageError(err error) bool {
	for _, target := range []error{
		usecase.ErrEmptyPrompt,
		domain.ErrAgentNotFound,
		domain.ErrSessionNotFound,
		domain.ErrSessionAgentMismatch,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func runAgent(ctx context.Context, configPath string, in usecase.RunAgentInput) (usecase.RunAgentOutput, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return usecase.RunAgentOutput{}, err
	}
	dockerClient, err := client.New(client.FromEnv)
	if err != nil {
		return usecase.RunAgentOutput{}, err
	}
	defer func() { _ = dockerClient.Close() }()

	issuer, err := githubIssuer(cfg.GitHubApps)
	if err != nil {
		return usecase.RunAgentOutput{}, err
	}

	uc := usecase.NewRunAgent(
		cfg.Agents,
		filesession.NewStore(cfg.SessionsRoot),
		gitworkspace.NewProvider(cfg.SessionsRoot),
		issuer,
		docker.NewRunner(dockerClient, docker.Options{ClaudeConfigVolume: cfg.Claude.ConfigVolume}),
	)
	return uc.Execute(ctx, in)
}

func githubIssuer(apps map[domain.AgentName]config.GitHubApp) (*githubapp.Issuer, error) {
	loaded := make(map[domain.AgentName]githubapp.App, len(apps))
	for name, app := range apps {
		key, err := githubapp.LoadPrivateKey(app.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("agent %q: %w", name, err)
		}
		loaded[name] = githubapp.App{ID: app.AppID, InstallationID: app.InstallationID, PrivateKey: key}
	}
	return githubapp.NewIssuer(loaded, githubapp.DefaultBaseURL, &http.Client{Timeout: 30 * time.Second}), nil
}
