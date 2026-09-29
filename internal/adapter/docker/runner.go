package docker

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// dockerAPI is the subset of *client.Client the runner uses.
type dockerAPI interface {
	ContainerCreate(ctx context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error)
	ContainerStart(ctx context.Context, containerID string, options client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerWait(ctx context.Context, containerID string, options client.ContainerWaitOptions) client.ContainerWaitResult
	ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error)
	ContainerRemove(ctx context.Context, containerID string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
}

type Options struct {
	ClaudeConfigVolume string
}

type Runner struct {
	api  dockerAPI
	opts Options
}

func NewRunner(api dockerAPI, opts Options) *Runner {
	return &Runner{api: api, opts: opts}
}

func (r *Runner) Run(ctx context.Context, agent domain.Agent, prompt string) (string, error) {
	created, err := r.api.ContainerCreate(ctx, containerSpec(agent, prompt, r.opts))
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}
	defer func() {
		_, _ = r.api.ContainerRemove(context.WithoutCancel(ctx), created.ID, client.ContainerRemoveOptions{Force: true})
	}()

	if _, err := r.api.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("start container: %w", err)
	}

	wait := r.api.ContainerWait(ctx, created.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	var status int64
	select {
	case res := <-wait.Result:
		status = res.StatusCode
	case err := <-wait.Error:
		return "", fmt.Errorf("wait for container: %w", err)
	}

	logs, err := r.api.ContainerLogs(ctx, created.ID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", fmt.Errorf("read container logs: %w", err)
	}
	defer func() { _ = logs.Close() }()
	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, logs); err != nil {
		return "", fmt.Errorf("read container logs: %w", err)
	}

	if status != 0 {
		return "", fmt.Errorf("agent exited with status %d: %s", status, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

const claudeConfigDir = "/home/node/.claude"

func containerSpec(agent domain.Agent, prompt string, opts Options) client.ContainerCreateOptions {
	workdir := path.Join("/workspace", filepath.Base(agent.Target.Path))
	return client.ContainerCreateOptions{
		Config: &container.Config{
			Image: agent.Target.Image,
			// Relies on the target image contract in docs/target/README.md.
			Cmd: []string{
				"runuser", "-u", "node", "--",
				"claude", "-p", prompt, "--dangerously-skip-permissions",
			},
			Env: []string{
				"WORKSPACE_DIR=" + workdir,
				"CLAUDE_CONFIG_DIR=" + claudeConfigDir,
				"FIREWALL_ENABLED=false",
			},
			WorkingDir: workdir,
		},
		HostConfig: &container.HostConfig{
			Mounts: []mount.Mount{
				{Type: mount.TypeBind, Source: agent.Target.Path, Target: workdir},
				{Type: mount.TypeVolume, Source: opts.ClaudeConfigVolume, Target: claudeConfigDir},
			},
		},
	}
}
