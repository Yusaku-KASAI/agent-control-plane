package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

var testAgent = domain.Agent{
	Name:   "repo-dev",
	Role:   "developer",
	Target: domain.Target{Path: "/src/repo", Image: "repo-dev:latest"},
}

var testSession = domain.Session{
	ID:        "0f8fad5b-d9cb-469f-a165-70867728950e",
	AgentName: "repo-dev",
	Workspace: domain.Workspace{Path: "/sessions/0f8fad5b/workspace/repo"},
}

func TestContainerSpec(t *testing.T) {
	spec := containerSpec(usecase.Turn{Agent: testAgent, Session: testSession, Prompt: "fix it"}, Options{ClaudeConfigVolume: "claude-config"})

	if spec.Config.Image != "repo-dev:latest" {
		t.Errorf("Image = %q", spec.Config.Image)
	}
	if spec.Config.WorkingDir != "/workspace/repo" {
		t.Errorf("WorkingDir = %q", spec.Config.WorkingDir)
	}
	wantCmd := []string{
		"runuser", "-u", "node", "--",
		"claude", "-p", "fix it",
		"--agent", "developer",
		"--session-id", "0f8fad5b-d9cb-469f-a165-70867728950e",
		"--dangerously-skip-permissions",
	}
	if !slices.Equal(spec.Config.Cmd, wantCmd) {
		t.Errorf("Cmd = %q, want %q", spec.Config.Cmd, wantCmd)
	}
	if !slices.Contains(spec.Config.Env, "ACP_SESSION_BRANCH=acp/0f8fad5b") {
		t.Errorf("Env = %q, want ACP_SESSION_BRANCH", spec.Config.Env)
	}
	wantMounts := []mount.Mount{
		{Type: mount.TypeBind, Source: "/sessions/0f8fad5b/workspace/repo", Target: "/workspace/repo"},
		{Type: mount.TypeVolume, Source: "claude-config", Target: "/home/node/.claude"},
	}
	if !slices.Equal(spec.HostConfig.Mounts, wantMounts) {
		t.Errorf("Mounts = %+v, want %+v", spec.HostConfig.Mounts, wantMounts)
	}
}

func TestContainerSpec_ResumesStartedSession(t *testing.T) {
	started := testSession
	started.Turns = 1
	cmd := containerSpec(usecase.Turn{Agent: testAgent, Session: started, Prompt: "next"}, Options{}).Config.Cmd
	i := slices.Index(cmd, "--resume")
	if i < 0 || cmd[i+1] != string(started.ID) || slices.Contains(cmd, "--session-id") {
		t.Errorf("Cmd = %q, want --resume %s", cmd, started.ID)
	}
}

func TestContainerSpec_Credentials(t *testing.T) {
	agent := testAgent
	agent.Git = domain.GitIdentity{Name: "repo-dev (agent)", Email: "1+u@users.noreply.github.com"}
	turn := usecase.Turn{Agent: agent, Session: testSession, Prompt: "p", Credentials: domain.Credentials{GitHubToken: "ghs_x"}}

	env := containerSpec(turn, Options{}).Config.Env
	for _, want := range []string{
		"GIT_AUTHOR_NAME=repo-dev (agent)",
		"GIT_COMMITTER_EMAIL=1+u@users.noreply.github.com",
		"GH_TOKEN=ghs_x",
		"GIT_CONFIG_COUNT=3",
		"GIT_CONFIG_VALUE_0=!gh auth git-credential",
	} {
		if !slices.Contains(env, want) {
			t.Errorf("Env lacks %q: %q", want, env)
		}
	}

	bare := containerSpec(usecase.Turn{Agent: testAgent, Session: testSession, Prompt: "p"}, Options{}).Config.Env
	for _, e := range bare {
		if strings.HasPrefix(e, "GH_TOKEN=") || strings.HasPrefix(e, "GIT_") {
			t.Errorf("Env without credentials has %q", e)
		}
	}
}

func TestRunner_Run(t *testing.T) {
	tests := []struct {
		name       string
		status     int64
		stdout     string
		stderr     string
		want       string
		wantErrSub string
	}{
		{name: "success", stdout: "done\n", stderr: "noise", want: "done\n"},
		{name: "non-zero exit", status: 1, stderr: "boom\n", wantErrSub: "status 1: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{status: tt.status, logs: multiplex(tt.stdout, tt.stderr)}
			got, err := NewRunner(api, Options{ClaudeConfigVolume: "v"}).Run(context.Background(), usecase.Turn{Agent: testAgent, Session: testSession, Prompt: "hi"})

			if tt.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("Run() error = %v, want containing %q", err, tt.wantErrSub)
				}
			} else if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Run() = %q, want %q", got, tt.want)
			}
			if !api.removed {
				t.Error("container was not removed")
			}
		})
	}
}

type fakeAPI struct {
	status  int64
	logs    []byte
	removed bool
}

func (f *fakeAPI) ContainerCreate(context.Context, client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	return client.ContainerCreateResult{ID: "c1"}, nil
}

func (f *fakeAPI) ContainerStart(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error) {
	return client.ContainerStartResult{}, nil
}

func (f *fakeAPI) ContainerWait(context.Context, string, client.ContainerWaitOptions) client.ContainerWaitResult {
	res := make(chan container.WaitResponse, 1)
	res <- container.WaitResponse{StatusCode: f.status}
	return client.ContainerWaitResult{Result: res, Error: make(chan error)}
}

func (f *fakeAPI) ContainerLogs(context.Context, string, client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	return io.NopCloser(bytes.NewReader(f.logs)), nil
}

func (f *fakeAPI) ContainerRemove(context.Context, string, client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	f.removed = true
	return client.ContainerRemoveResult{}, nil
}

// multiplex encodes stdout/stderr in Docker's non-TTY log stream framing.
func multiplex(stdout, stderr string) []byte {
	var buf bytes.Buffer
	for _, f := range []struct {
		stream byte
		data   string
	}{{1, stdout}, {2, stderr}} {
		if f.data == "" {
			continue
		}
		header := make([]byte, 8)
		header[0] = f.stream
		binary.BigEndian.PutUint32(header[4:], uint32(len(f.data)))
		buf.Write(header)
		buf.WriteString(f.data)
	}
	return buf.Bytes()
}
