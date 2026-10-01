package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/moby/moby/client"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/config"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/docker"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/filesession"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/githubapp"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/gitworkspace"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/httpapi"
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

	turns := usecase.NewRunAgent(
		cfg.Agents,
		filesession.NewStore(cfg.SessionsRoot),
		gitworkspace.NewProvider(cfg.SessionsRoot),
		issuer,
		docker.NewRunner(dockerClient, docker.Options{ClaudeConfigVolume: cfg.Claude.ConfigVolume}),
	)
	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewHandler(turns, token, log),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "addr", addr)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
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
