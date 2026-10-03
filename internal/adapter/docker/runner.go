package docker

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strconv"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// dockerAPI is the subset of *client.Client the runner uses.
type dockerAPI interface {
	ContainerCreate(ctx context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error)
	ContainerAttach(ctx context.Context, containerID string, options client.ContainerAttachOptions) (client.ContainerAttachResult, error)
	ContainerStart(ctx context.Context, containerID string, options client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerWait(ctx context.Context, containerID string, options client.ContainerWaitOptions) client.ContainerWaitResult
	ContainerRemove(ctx context.Context, containerID string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
}

type Options struct {
	ClaudeConfigVolume string
	// CredentialsDir is where the runner writes each session's credentials.
	CredentialsDir string
	// CredentialsVolume is the Docker volume mounted at CredentialsDir; agent containers get
	// their session's subpath of it. When empty, CredentialsDir is a host path and is bind-mounted.
	CredentialsVolume string
}

type Runner struct {
	api  dockerAPI
	opts Options
}

func NewRunner(api dockerAPI, opts Options) *Runner {
	return &Runner{api: api, opts: opts}
}

func (r *Runner) Start(ctx context.Context, req usecase.StartRequest) (usecase.Process, error) {
	plugins, err := declaredPluginDirs(req.Session.Workspace.Path)
	if err != nil {
		return nil, fmt.Errorf("read declared plugins: %w", err)
	}
	creds := sessionCredentials{root: r.opts.CredentialsDir, id: req.Session.ID}
	if err := creds.write(req.Credentials); err != nil {
		return nil, fmt.Errorf("write credentials: %w", err)
	}

	created, err := r.api.ContainerCreate(ctx, containerSpec(req, plugins, r.opts))
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}
	remove := func() {
		_, _ = r.api.ContainerRemove(context.WithoutCancel(ctx), created.ID, client.ContainerRemoveOptions{Force: true})
	}
	attach, err := r.api.ContainerAttach(ctx, created.ID, client.ContainerAttachOptions{Stream: true, Stdin: true, Stdout: true, Stderr: true})
	if err != nil {
		remove()
		return nil, fmt.Errorf("attach container: %w", err)
	}
	if _, err := r.api.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		attach.Close()
		remove()
		return nil, fmt.Errorf("start container: %w", err)
	}
	return newProcess(r.api, created.ID, attach.HijackedResponse, creds), nil
}

const (
	claudeConfigDir = "/home/node/.claude"
	credentialsDir  = "/run/acp/credentials"
)

func containerSpec(req usecase.StartRequest, plugins []string, opts Options) client.ContainerCreateOptions {
	agent, s := req.Agent, req.Session
	// Claude Code keys conversation history by working directory, so it must stay fixed per session.
	workdir := path.Join("/workspace", filepath.Base(agent.Target.Path))
	return client.ContainerCreateOptions{
		Config: &container.Config{
			Image: agent.Target.Image,
			// Relies on the target image contract in docs/target/README.md.
			Cmd: claudeCommand(agent, s, workdir, plugins),
			Env: append([]string{
				"WORKSPACE_DIR=" + workdir,
				"CLAUDE_CONFIG_DIR=" + claudeConfigDir,
				"FIREWALL_ENABLED=false",
				"ACP_SESSION_BRANCH=" + s.ID.Branch(),
			}, credentialEnv(agent.Git, req.Credentials)...),
			WorkingDir: workdir,
			// Prompts are written to stdin; closing it ends the process.
			OpenStdin:    true,
			StdinOnce:    true,
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
		},
		HostConfig: &container.HostConfig{
			Mounts: []mount.Mount{
				{Type: mount.TypeBind, Source: s.Workspace.Path, Target: workdir},
				{Type: mount.TypeVolume, Source: opts.ClaudeConfigVolume, Target: claudeConfigDir},
				credentialsMount(s.ID, opts),
			},
		},
	}
}

func claudeCommand(agent domain.Agent, s domain.Session, workdir string, plugins []string) []string {
	sessionFlag := "--resume"
	if s.IsNew() {
		sessionFlag = "--session-id"
	}
	cmd := []string{
		"runuser", "-u", "node", "--",
		"claude", "-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--replay-user-messages",
		"--verbose",
		"--agent", string(agent.Role),
		sessionFlag, string(s.ID),
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
		// gh reads the token from GH_CONFIG_DIR on every call, so refreshed tokens take effect;
		// git authenticates through gh, over HTTPS even for SSH-style remotes.
		env = append(env, "GH_CONFIG_DIR="+path.Join(credentialsDir, "gh"), "GH_NO_UPDATE_NOTIFIER=1")
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
