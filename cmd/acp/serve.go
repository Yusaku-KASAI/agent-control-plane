package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/client"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/config"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/docker"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/filesession"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/githubapp"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/gitworkspace"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/httpapi"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/slackbot"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

func serve(ctx context.Context, args []string, getenv func(string) string, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "acp.yaml", "config file path")
	listen := fs.String("listen", "127.0.0.1:8787", "address to listen on")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	token := getenv("ACP_API_TOKEN")
	if token == "" {
		fmt.Fprintln(stderr, "acp: ACP_API_TOKEN is not set")
		return 2
	}

	log := slog.New(slog.NewTextHandler(stderr, nil))
	if err := listenAndServe(ctx, *configPath, *listen, token, log); err != nil {
		log.Error("server stopped", "err", err)
		return 1
	}
	return 0
}

func listenAndServe(ctx context.Context, configPath, addr, token string, log *slog.Logger) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	dockerClient, err := client.New(client.FromEnv)
	if err != nil {
		return err
	}
	defer func() { _ = dockerClient.Close() }()
	issuer, err := githubIssuer(cfg.GitHubApps)
	if err != nil {
		return err
	}

	notifier := &agentTurnNotifier{log: log}
	turns := usecase.NewDispatcher(
		cfg.Agents,
		filesession.NewStore(cfg.SessionsRoot),
		gitworkspace.NewProvider(cfg.SessionsRoot),
		issuer,
		docker.NewRunner(dockerClient, docker.Options{
			ClaudeConfigVolume: cfg.Claude.ConfigVolume,
			CredentialsDir:     cfg.Credentials.Dir,
			CredentialsVolume:  cfg.Credentials.Volume,
		}),
		notifier,
		usecase.DispatcherOptions{
			IdleTimeout: cfg.IdleTimeout,
			// Installation tokens live for an hour.
			CredentialRefresh: 40 * time.Minute,
			StopTimeout:       30 * time.Second,
		},
	)
	// Runs after the HTTP server and the bots stop, so no new prompt arrives while processes close.
	defer turns.Close()
	bots, err := slackBots(cfg, turns, log)
	if err != nil {
		return err
	}
	notifier.setBots(bots)
	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewHandler(turns, token, log),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errc := make(chan error, 1+len(bots))
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "addr", addr)
	for _, b := range bots {
		go func() { errc <- b.Run(ctx) }()
	}

	var runErr error
	select {
	case runErr = <-errc:
		cancel()
	case <-ctx.Done():
	}
	shutdownCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer stop()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	if errors.Is(runErr, http.ErrServerClosed) || errors.Is(runErr, context.Canceled) {
		return nil
	}
	return runErr
}

func slackBots(cfg config.Config, turns usecase.TurnExecutor, log *slog.Logger) (map[domain.AgentName]*slackbot.Bot, error) {
	bots := make(map[domain.AgentName]*slackbot.Bot, len(cfg.SlackApps))
	for name, app := range cfg.SlackApps {
		appToken, err := readSecret(app.AppTokenPath)
		if err != nil {
			return nil, fmt.Errorf("agent %q: %w", name, err)
		}
		botToken, err := readSecret(app.BotTokenPath)
		if err != nil {
			return nil, fmt.Errorf("agent %q: %w", name, err)
		}
		bots[name] = slackbot.New(name, appToken, botToken, turns, cfg.Access, log)
	}
	return bots, nil
}

func readSecret(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
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

// agentTurnNotifier posts turns the agent started by itself to the Slack thread the session
// came from. Sessions started elsewhere have no thread to post to, so the result is logged.
type agentTurnNotifier struct {
	log  *slog.Logger
	mu   sync.Mutex
	bots map[domain.AgentName]*slackbot.Bot
}

func (n *agentTurnNotifier) setBots(bots map[domain.AgentName]*slackbot.Bot) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.bots = bots
}

func (n *agentTurnNotifier) AgentTurn(ctx context.Context, s domain.Session, r usecase.TurnResult) {
	n.mu.Lock()
	bot, ok := n.bots[s.AgentName]
	n.mu.Unlock()
	if !ok || s.Conversation.Platform != "slack" {
		n.log.Info("agent turn without a conversation", "agent", s.AgentName, "session", s.ID, "output", r.Output)
		return
	}
	if err := bot.Post(ctx, s.Conversation, r.Output); err != nil {
		n.log.Warn("posting agent turn failed", "agent", s.AgentName, "session", s.ID, "err", err)
	}
}
