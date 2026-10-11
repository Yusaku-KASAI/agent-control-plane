package docker

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
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
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
}

type Options struct {
	// ClaudeConfigDir holds a Claude Code config directory per agent: its conversation history and
	// auto memory. When ClaudeConfigVolume is set, that volume is mounted at ClaudeConfigDir and
	// agents get their subpath of it; otherwise ClaudeConfigDir is a host path and is bind-mounted.
	ClaudeConfigDir    string
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
	plugins, err := declaredPlugins(req.Session.Workspace.Path, filepath.Dir(req.Agent.Target.Path), workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("read declared plugins: %w", err)
	}
	creds := sessionCredentials{root: r.opts.CredentialsDir, id: req.Session.ID}
	if err := creds.write(req.Credentials); err != nil {
		return nil, fmt.Errorf("write credentials: %w", err)
	}
	// A volume subpath must exist before it is mounted.
	if err := os.MkdirAll(filepath.Join(r.opts.ClaudeConfigDir, string(req.Agent.Name)), 0o755); err != nil {
		return nil, fmt.Errorf("create claude config dir: %w", err)
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

// RemoveLeftovers removes every agent container, running or not, and returns their sessions.
func (r *Runner) RemoveLeftovers(ctx context.Context) ([]domain.SessionID, error) {
	list, err := r.api.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("label", sessionLabel),
	})
	if err != nil {
		return nil, fmt.Errorf("list agent containers: %w", err)
	}
	var ids []domain.SessionID
	for _, c := range list.Items {
		if _, err := r.api.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: true}); err != nil {
			return ids, fmt.Errorf("remove container %s: %w", c.ID, err)
		}
		if id := domain.SessionID(c.Labels[sessionLabel]); !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

const (
	sessionLabel = "acp.session"
	agentLabel   = "acp.agent"
)

const (
	// workspaceDir holds the repository in the container, as in the devcontainer, where the
	// repositories beside it are mounted too.
	workspaceDir    = "/workspace"
	claudeConfigDir = "/home/node/.claude"
	credentialsDir  = "/run/acp/credentials"
)

func containerSpec(req usecase.StartRequest, plugins []pluginDir, opts Options) client.ContainerCreateOptions {
	agent, s := req.Agent, req.Session
	// Claude Code keys conversation history by working directory, so it must stay fixed per session.
	workdir := path.Join(workspaceDir, filepath.Base(agent.Target.Path))
	pluginPaths, besideMounts := pluginMounts(plugins, workdir)
	return client.ContainerCreateOptions{
		Config: &container.Config{
			Image: agent.Target.Image,
			// Relies on the target image contract in docs/target/contract.md.
			Cmd: claudeCommand(agent, s, pluginPaths),
			Env: append([]string{
				"CLAUDE_CONFIG_DIR=" + claudeConfigDir,
				"ACP_SESSION_BRANCH=" + s.ID.Branch(),
				"ACP_CO_AUTHORS_FILE=" + path.Join(credentialsDir, coAuthorsFile),
			}, credentialEnv(agent.Git, req.Credentials)...),
			WorkingDir: workdir,
			Labels:     map[string]string{sessionLabel: string(s.ID), agentLabel: string(agent.Name)},
			// Prompts are written to stdin; closing it ends the process.
			OpenStdin:    true,
			StdinOnce:    true,
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
		},
		HostConfig: &container.HostConfig{
			Mounts: append([]mount.Mount{
				{Type: mount.TypeBind, Source: s.Workspace.Path, Target: workdir},
				claudeConfigMount(agent.Name, opts),
				credentialsMount(s.ID, opts),
			}, besideMounts...),
		},
	}
}

// pluginMounts returns where each plugin is in the container, and read-only mounts of only the
// marketplaces beside the repository that they come from.
func pluginMounts(plugins []pluginDir, workdir string) ([]string, []mount.Mount) {
	var paths []string
	var mounts []mount.Mount
	for _, p := range plugins {
		if p.Beside == nil {
			paths = append(paths, path.Join(workdir, filepath.ToSlash(p.Path)))
			continue
		}
		paths = append(paths, p.Path)
		if !slices.ContainsFunc(mounts, func(m mount.Mount) bool { return m.Target == p.Beside.Container }) {
			mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: p.Beside.Host, Target: p.Beside.Container, ReadOnly: true})
		}
	}
	return paths, mounts
}

func claudeCommand(agent domain.Agent, s domain.Session, plugins []string) []string {
	sessionFlag := "--resume"
	if s.IsNew() {
		sessionFlag = "--session-id"
	}
	cmd := []string{
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
		cmd = append(cmd, "--plugin-dir", p)
	}
	return cmd
}

func claudeConfigMount(agent domain.AgentName, opts Options) mount.Mount {
	if opts.ClaudeConfigVolume != "" {
		return mount.Mount{
			Type: mount.TypeVolume, Source: opts.ClaudeConfigVolume, Target: claudeConfigDir,
			VolumeOptions: &mount.VolumeOptions{Subpath: string(agent)},
		}
	}
	return mount.Mount{Type: mount.TypeBind, Source: filepath.Join(opts.ClaudeConfigDir, string(agent)), Target: claudeConfigDir}
}

func credentialEnv(git domain.GitIdentity, creds domain.Credentials) []string {
	var env []string
	switch {
	case creds.ClaudeOAuthToken != "":
		env = append(env, "CLAUDE_CODE_OAUTH_TOKEN="+creds.ClaudeOAuthToken)
	case creds.ClaudeAPIKey != "":
		env = append(env, "ANTHROPIC_API_KEY="+creds.ClaudeAPIKey)
	}
	if git.Name != "" {
		env = append(env,
			"GIT_AUTHOR_NAME="+git.Name, "GIT_AUTHOR_EMAIL="+git.Email,
			"GIT_COMMITTER_NAME="+git.Name, "GIT_COMMITTER_EMAIL="+git.Email,
		)
	}
	if creds.SlackBotToken != "" {
		// Read on every call by the slack plugin, so a refreshed token takes effect.
		env = append(env, "SLACK_BOT_TOKEN_FILE="+path.Join(credentialsDir, slackTokenFile))
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
