package docker

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
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

func (r *Runner) Run(ctx context.Context, t usecase.Turn) (string, error) {
	plugins, err := declaredPluginDirs(t.Session.Workspace.Path)
	if err != nil {
		return "", fmt.Errorf("read declared plugins: %w", err)
	}
	created, err := r.api.ContainerCreate(ctx, containerSpec(t, plugins, r.opts))
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

func containerSpec(t usecase.Turn, plugins []string, opts Options) client.ContainerCreateOptions {
	agent, s := t.Agent, t.Session
	// Claude Code keys conversation history by working directory, so it must stay fixed per session.
	workdir := path.Join("/workspace", filepath.Base(agent.Target.Path))
	sessionFlag := "--resume"
	if s.IsNew() {
		sessionFlag = "--session-id"
	}
	return client.ContainerCreateOptions{
		Config: &container.Config{
			Image: agent.Target.Image,
			// Relies on the target image contract in docs/target/README.md.
			Cmd: claudeCommand(t, workdir, plugins, sessionFlag),
			Env: append([]string{
				"WORKSPACE_DIR=" + workdir,
				"CLAUDE_CONFIG_DIR=" + claudeConfigDir,
				"FIREWALL_ENABLED=false",
				"ACP_SESSION_BRANCH=" + s.ID.Branch(),
			}, credentialEnv(agent.Git, t.Credentials)...),
			WorkingDir: workdir,
		},
		HostConfig: &container.HostConfig{
			Mounts: []mount.Mount{
				{Type: mount.TypeBind, Source: s.Workspace.Path, Target: workdir},
				{Type: mount.TypeVolume, Source: opts.ClaudeConfigVolume, Target: claudeConfigDir},
			},
		},
	}
}

func claudeCommand(t usecase.Turn, workdir string, plugins []string, sessionFlag string) []string {
	cmd := []string{
		"runuser", "-u", "node", "--",
		"claude", "-p", t.Prompt,
		"--agent", string(t.Agent.Role),
		sessionFlag, string(t.Session.ID),
		"--dangerously-skip-permissions",
	}
	for _, p := range plugins {
		cmd = append(cmd, "--plugin-dir", path.Join(workdir, filepath.ToSlash(p)))
	}
	return cmd
}

func credentialEnv(git domain.GitIdentity, creds domain.Credentials) []string {
	var env []string
	if git.Name != "" {
		env = append(env,
			"GIT_AUTHOR_NAME="+git.Name, "GIT_AUTHOR_EMAIL="+git.Email,
			"GIT_COMMITTER_NAME="+git.Name, "GIT_COMMITTER_EMAIL="+git.Email,
		)
	}
	if creds.GitHubToken != "" {
		// gh reads GH_TOKEN; git authenticates through gh, over HTTPS even for SSH-style remotes.
		env = append(env, "GH_TOKEN="+creds.GitHubToken)
		env = append(env, gitConfigEnv(
			[2]string{"credential.https://github.com.helper", "!gh auth git-credential"},
			[2]string{"url.https://github.com/.insteadOf", "git@github.com:"},
			[2]string{"url.https://github.com/.insteadOf", "ssh://git@github.com/"},
		)...)
	}
	return env
}

func gitConfigEnv(entries ...[2]string) []string {
	env := []string{"GIT_CONFIG_COUNT=" + strconv.Itoa(len(entries))}
	for i, kv := range entries {
		env = append(env,
			fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", i, kv[0]),
			fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", i, kv[1]),
		)
	}
	return env
}
