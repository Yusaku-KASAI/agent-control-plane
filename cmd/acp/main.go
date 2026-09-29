package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/moby/moby/client"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/config"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/docker"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

const usage = `usage: acp run --agent <name> [--config <path>] <prompt>`

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
	configPath := fs.String("config", "acp.yaml", "config file path")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *agentName == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}

	out, err := runAgent(ctx, *configPath, domain.AgentName(*agentName), strings.Join(fs.Args(), " "))
	if err != nil {
		fmt.Fprintln(stderr, "acp:", err)
		if errors.Is(err, usecase.ErrEmptyPrompt) || errors.Is(err, domain.ErrAgentNotFound) {
			return 2
		}
		return 1
	}
	fmt.Fprint(stdout, out)
	return 0
}

func runAgent(ctx context.Context, configPath string, name domain.AgentName, prompt string) (string, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return "", err
	}
	dockerClient, err := client.New(client.FromEnv)
	if err != nil {
		return "", err
	}
	defer func() { _ = dockerClient.Close() }()

	runner := docker.NewRunner(dockerClient, docker.Options{ClaudeConfigVolume: cfg.Claude.ConfigVolume})
	out, err := usecase.NewRunAgent(cfg.Agents, runner).Execute(ctx, usecase.RunAgentInput{AgentName: name, Prompt: prompt})
	return out.Output, err
}
