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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/client"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/claudeauth"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/config"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/docker"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/filesession"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/githubapp"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/gitworkspace"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/httpapi"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/slackauth"
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
	github, err := githubIssuer(cfg.GitHubApps)
	if err != nil {
		return err
	}
	slackTokens, rotating, err := slackTokenSources(cfg)
	if err != nil {
		return err
	}
	issuer := credentialIssuers{
		claudeauth.FileIssuer{OAuthTokenPath: cfg.Claude.OAuthTokenPath, APIKeyPath: cfg.Claude.APIKeyPath},
		github,
		slackauth.NewIssuer(slackTokens),
	}

	observer := &sessionObserver{log: log}
	turns := usecase.NewDispatcher(
		cfg.Agents,
		filesession.NewStore(cfg.SessionsRoot),
		gitworkspace.NewProvider(cfg.SessionsRoot),
		issuer,
		cfg.People,
		docker.NewRunner(dockerClient, docker.Options{
			ClaudeConfigDir:    cfg.Claude.ConfigDir,
			ClaudeConfigVolume: cfg.Claude.ConfigVolume,
			CredentialsDir:     cfg.Credentials.Dir,
			CredentialsVolume:  cfg.Credentials.Volume,
		}),
		observer,
		usecase.DispatcherOptions{
			IdleTimeout: cfg.IdleTimeout,
			// Installation tokens live for an hour.
			CredentialRefresh: 40 * time.Minute,
			StopTimeout:       30 * time.Second,
		},
	)
	bots, err := slackBots(cfg, slackTokens, turns, log)
	if err != nil {
		turns.Close()
		return err
	}
	observer.setBots(bots)
	if n, err := turns.Recover(ctx); err != nil {
		log.Error("cleaning up after the previous run failed", "containers", n, "err", err)
	} else if n > 0 {
		log.Info("removed agent containers left by the previous run", "containers", n)
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewHandler(turns, token, log),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "addr", addr)
	// A bot that cannot run, such as one whose tokens are missing, stops alone; the other agents
	// and the API keep serving.
	var running sync.WaitGroup
	for name, b := range bots {
		running.Go(func() {
			if err := b.Run(ctx); err != nil && ctx.Err() == nil {
				log.Error("slack bot stopped", "agent", name, "err", err)
			}
		})
	}
	for name, src := range rotating {
		go src.Run(ctx, 10*time.Minute, func(err error) {
			log.Error("slack token refresh failed", "agent", name, "err", err)
		})
	}

	var runErr error
	select {
	case runErr = <-errc:
		cancel()
	case <-ctx.Done():
	}
	// The bots stopped taking requests with ctx and the dispatcher refuses new ones once closed;
	// closing it ends the waiting requests, which lets the HTTP server and the bots finish.
	turns.Close()
	shutdownCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer stop()
	shutdownErr := srv.Shutdown(shutdownCtx)
	running.Wait()
	for _, b := range bots {
		b.Wait()
	}
	if shutdownErr != nil && !errors.Is(shutdownErr, http.ErrServerClosed) {
		return shutdownErr
	}
	if errors.Is(runErr, http.ErrServerClosed) || errors.Is(runErr, context.Canceled) {
		return nil
	}
	return runErr
}

func slackBots(cfg config.Config, tokens map[domain.AgentName]slackauth.Source, turns usecase.TurnExecutor, log *slog.Logger) (map[domain.AgentName]*slackbot.Bot, error) {
	bots := make(map[domain.AgentName]*slackbot.Bot, len(cfg.SlackApps))
	for name, app := range cfg.SlackApps {
		appToken, err := readSecret(app.AppTokenPath)
		if err != nil {
			return nil, fmt.Errorf("agent %q: %w", name, err)
		}
		bots[name] = slackbot.New(name, appToken, tokens[name], turns, cfg.Access, log)
	}
	return bots, nil
}

// slackTokenSources gives each agent with a Slack app its bot token source; the rotating ones
// are also returned so that they can be refreshed in the background.
func slackTokenSources(cfg config.Config) (map[domain.AgentName]slackauth.Source, map[domain.AgentName]*slackauth.RotatingSource, error) {
	sources := make(map[domain.AgentName]slackauth.Source, len(cfg.SlackApps))
	rotating := make(map[domain.AgentName]*slackauth.RotatingSource)
	for name, app := range cfg.SlackApps {
		if app.Rotation == nil {
			sources[name] = slackauth.FileSource(app.BotTokenPath)
			continue
		}
		secret, err := readSecret(app.Rotation.ClientSecretPath)
		if err != nil {
			return nil, nil, fmt.Errorf("agent %q: %w", name, err)
		}
		src := slackauth.NewRotatingSource(slackauth.RotatingConfig{
			ClientID:         app.Rotation.ClientID,
			ClientSecret:     secret,
			InitialTokenPath: app.BotTokenPath,
			StatePath:        filepath.Join(cfg.StateDir, "slack", string(name)+".json"),
			BaseURL:          slackauth.DefaultBaseURL,
		}, &http.Client{Timeout: 30 * time.Second})
		sources[name], rotating[name] = src, src
	}
	return sources, rotating, nil
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

// sessionObserver hands session events to the Slack bot of the session's agent. Sessions
// started elsewhere have no thread to follow, so the agent's own turns are only logged.
type sessionObserver struct {
	log  *slog.Logger
	mu   sync.Mutex
	bots map[domain.AgentName]*slackbot.Bot
}

func (o *sessionObserver) setBots(bots map[domain.AgentName]*slackbot.Bot) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.bots = bots
}

func (o *sessionObserver) Observe(s domain.Session, e usecase.SessionEvent) {
	o.mu.Lock()
	bot, ok := o.bots[s.AgentName]
	o.mu.Unlock()
	if ok && s.Conversation.Platform == "slack" {
		bot.Observe(s, e)
		return
	}
	if e.Kind == usecase.SessionTurnFinished && e.AgentStarted {
		o.log.Info("agent turn without a conversation", "agent", s.AgentName, "session", s.ID, "output", e.Result.Output)
	}
}

// credentialIssuers merges what each issuer grants the agent.
type credentialIssuers []usecase.CredentialIssuer

func (c credentialIssuers) Issue(ctx context.Context, agent domain.Agent) (domain.Credentials, error) {
	var merged domain.Credentials
	for _, issuer := range c {
		creds, err := issuer.Issue(ctx, agent)
		if err != nil {
			return domain.Credentials{}, err
		}
		if creds.GitHubToken != "" {
			merged.GitHubToken = creds.GitHubToken
		}
		if creds.SlackBotToken != "" {
			merged.SlackBotToken = creds.SlackBotToken
		}
		if creds.ClaudeOAuthToken != "" {
			merged.ClaudeOAuthToken = creds.ClaudeOAuthToken
		}
		if creds.ClaudeAPIKey != "" {
			merged.ClaudeAPIKey = creds.ClaudeAPIKey
		}
	}
	return merged, nil
}
