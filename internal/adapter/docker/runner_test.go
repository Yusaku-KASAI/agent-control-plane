package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

var (
	testAgent = domain.Agent{
		Name:   "repo-dev",
		Role:   "developer",
		Target: domain.Target{Path: "/src/repo", Image: "repo-dev:latest"},
	}
	testSession = domain.Session{
		ID:        "0f8fad5b-d9cb-469f-a165-70867728950e",
		AgentName: "repo-dev",
		Workspace: domain.Workspace{Path: "/sessions/0f8fad5b/workspace/repo"},
	}
)

func TestContainerSpec(t *testing.T) {
	spec := containerSpec(usecase.StartRequest{Agent: testAgent, Session: testSession},
		[]string{"platform/plugins/roles"}, Options{ClaudeConfigVolume: "claude-config", CredentialsDir: "/var/lib/acp/credentials", CredentialsVolume: "acp-credentials"})

	if spec.Config.Image != "repo-dev:latest" || spec.Config.WorkingDir != "/workspace/repo" {
		t.Errorf("Image = %q, WorkingDir = %q", spec.Config.Image, spec.Config.WorkingDir)
	}
	if !spec.Config.OpenStdin || !spec.Config.StdinOnce || !spec.Config.AttachStdin {
		t.Error("stdin is not open for prompts")
	}
	wantCmd := []string{
		"runuser", "-u", "node", "--",
		"claude", "-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--replay-user-messages",
		"--verbose",
		"--agent", "developer",
		"--session-id", "0f8fad5b-d9cb-469f-a165-70867728950e",
		"--dangerously-skip-permissions",
		"--plugin-dir", "/workspace/repo/platform/plugins/roles",
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
		{
			Type: mount.TypeVolume, Source: "acp-credentials", Target: "/run/acp/credentials", ReadOnly: true,
			VolumeOptions: &mount.VolumeOptions{Subpath: "0f8fad5b-d9cb-469f-a165-70867728950e"},
		},
	}
	if len(spec.HostConfig.Mounts) != len(wantMounts) {
		t.Fatalf("Mounts = %+v", spec.HostConfig.Mounts)
	}
	for i, m := range spec.HostConfig.Mounts {
		if m.Type != wantMounts[i].Type || m.Source != wantMounts[i].Source || m.Target != wantMounts[i].Target ||
			m.ReadOnly != wantMounts[i].ReadOnly || (m.VolumeOptions == nil) != (wantMounts[i].VolumeOptions == nil) ||
			(m.VolumeOptions != nil && m.VolumeOptions.Subpath != wantMounts[i].VolumeOptions.Subpath) {
			t.Errorf("Mounts[%d] = %+v, want %+v", i, m, wantMounts[i])
		}
	}
}

func TestContainerSpec_ResumesStartedSession(t *testing.T) {
	started := testSession
	started.Turns = 1
	cmd := containerSpec(usecase.StartRequest{Agent: testAgent, Session: started}, nil, Options{}).Config.Cmd
	i := slices.Index(cmd, "--resume")
	if i < 0 || cmd[i+1] != string(started.ID) || slices.Contains(cmd, "--session-id") {
		t.Errorf("Cmd = %q, want --resume %s", cmd, started.ID)
	}
}

func TestContainerSpec_BindMountsCredentialsWithoutVolume(t *testing.T) {
	m := containerSpec(usecase.StartRequest{Agent: testAgent, Session: testSession}, nil, Options{CredentialsDir: "/host/creds"}).HostConfig.Mounts[2]
	if m.Type != mount.TypeBind || m.Source != "/host/creds/0f8fad5b-d9cb-469f-a165-70867728950e" || !m.ReadOnly {
		t.Errorf("credentials mount = %+v", m)
	}
}

func TestContainerSpec_Credentials(t *testing.T) {
	agent := testAgent
	agent.Git = domain.GitIdentity{Name: "repo-dev (agent)", Email: "1+u@users.noreply.github.com"}
	req := usecase.StartRequest{Agent: agent, Session: testSession, Credentials: domain.Credentials{GitHubToken: "ghs_x", SlackBotToken: "xoxb-x"}}

	env := containerSpec(req, nil, Options{}).Config.Env
	for _, want := range []string{
		"GIT_AUTHOR_NAME=repo-dev (agent)",
		"GIT_COMMITTER_EMAIL=1+u@users.noreply.github.com",
		"GH_CONFIG_DIR=/run/acp/credentials/gh",
		"SLACK_BOT_TOKEN_FILE=/run/acp/credentials/slack/bot-token",
		"GIT_CONFIG_COUNT=3",
		"GIT_CONFIG_VALUE_0=!gh auth git-credential",
	} {
		if !slices.Contains(env, want) {
			t.Errorf("Env lacks %q: %q", want, env)
		}
	}
	for _, e := range env {
		if strings.Contains(e, "ghs_x") || strings.Contains(e, "xoxb-x") {
			t.Errorf("the token is in the environment: %q", e)
		}
	}

	bare := containerSpec(usecase.StartRequest{Agent: testAgent, Session: testSession}, nil, Options{}).Config.Env
	for _, e := range bare {
		if strings.HasPrefix(e, "GH_") || strings.HasPrefix(e, "GIT_") || strings.HasPrefix(e, "SLACK_") {
			t.Errorf("Env without credentials has %q", e)
		}
	}
}

func TestSessionCredentials_Write(t *testing.T) {
	c := sessionCredentials{root: t.TempDir(), id: testSession.ID}
	for _, n := range []string{"first", "second"} {
		if err := c.write(domain.Credentials{GitHubToken: "ghs_" + n, SlackBotToken: "xoxb-" + n}); err != nil {
			t.Fatalf("write() error = %v", err)
		}
		b, err := os.ReadFile(filepath.Join(c.dir(), "gh", "hosts.yml"))
		if err != nil || !strings.Contains(string(b), "oauth_token: ghs_"+n) {
			t.Errorf("hosts.yml = %q, %v; want token ghs_%s", b, err, n)
		}
		b, err = os.ReadFile(filepath.Join(c.dir(), "slack", "bot-token"))
		if err != nil || string(b) != "xoxb-"+n {
			t.Errorf("slack bot-token = %q, %v; want xoxb-%s", b, err, n)
		}
	}
}

func TestRunner_Start(t *testing.T) {
	api := newFakeAPI()
	runner := NewRunner(api, Options{CredentialsDir: t.TempDir()})
	proc, err := runner.Start(context.Background(), usecase.StartRequest{Agent: testAgent, Session: testSession})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !api.started {
		t.Fatal("container was not started")
	}

	if err := proc.Send(context.Background(), "p1", "fix it"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	var msg struct {
		Type    string `json:"type"`
		UUID    string `json:"uuid"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(api.readStdin(t), &msg); err != nil || msg.Type != "user" || msg.UUID != "p1" || msg.Message.Content != "fix it" {
		t.Errorf("stdin got %+v, %v", msg, err)
	}

	api.emit(1, `{"type":"system","subtype":"init"}`+"\n")
	api.emit(1, `{"type":"user","isReplay":true,"uuid":"p1","message":{"role":"user","content":"fix it"}}`+"\n")
	api.emit(1, `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"ok"}]}}`+"\n")
	api.emit(1, `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"make check","description":"Run lint and tests"}}]}}`+"\n")
	api.emit(2, "some noise\n")
	api.emit(1, `{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"b1"}]}`+"\n")
	api.emit(1, `{"type":"result","subtype":"success","result":"done"}`+"\n")
	if e := receive(t, proc.Events()); e.Kind != usecase.EventTurnStarted {
		t.Errorf("event = %+v, want the turn start", e)
	}
	if e := receive(t, proc.Events()); e.Kind != usecase.EventPromptAccepted || e.PromptID != "p1" {
		t.Errorf("event = %+v, want prompt p1 accepted", e)
	}
	if e := receive(t, proc.Events()); e.Kind != usecase.EventActivity || e.Activity != "Bash: Run lint and tests" {
		t.Errorf("event = %+v, want the tool activity", e)
	}
	if e := receive(t, proc.Events()); e.Kind != usecase.EventBackgroundTasks || !slices.Equal(e.BackgroundTasks, []string{"b1"}) {
		t.Errorf("event = %+v, want one background task", e)
	}
	if e := receive(t, proc.Events()); e.Kind != usecase.EventTurnFinished || e.Result.Output != "done" || e.Result.Interrupted {
		t.Errorf("event = %+v, want the result", e)
	}

	if err := proc.Interrupt(context.Background()); err != nil {
		t.Fatalf("Interrupt() error = %v", err)
	}
	if line := api.readStdin(t); !bytes.Contains(line, []byte(`"subtype":"interrupt"`)) || !bytes.Contains(line, []byte(`"cancel_queued":true`)) {
		t.Errorf("stdin got %s, want an interrupt request", line)
	}
	if err := proc.StopTask(context.Background(), "b1"); err != nil {
		t.Fatalf("StopTask() error = %v", err)
	}
	if line := api.readStdin(t); !bytes.Contains(line, []byte(`"subtype":"stop_task"`)) || !bytes.Contains(line, []byte(`"task_id":"b1"`)) {
		t.Errorf("stdin got %s, want a stop_task request", line)
	}
	api.emit(1, `{"type":"result","subtype":"error_during_execution","result":""}`+"\n")
	if e := receive(t, proc.Events()); !e.Result.Interrupted {
		t.Errorf("event = %+v, want an interrupted result", e)
	}

	api.exit(1)
	if _, ok := <-proc.Events(); ok {
		t.Fatal("Events is still open after the process exited")
	}
	if err := proc.Err(); err == nil || !strings.Contains(err.Error(), "status 1: some noise") {
		t.Errorf("Err() = %v, want the exit status and stderr", err)
	}
	if err := proc.Close(context.Background()); err != nil || !api.removed {
		t.Errorf("Close() error = %v, removed = %v", err, api.removed)
	}
}

func TestStreamEventActivity(t *testing.T) {
	tests := map[string]string{
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Looking at the tests\nmore"}]}}`:                                   "Looking at the tests",
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/w/a.go"}}]}}`:                     "Edit: /w/a.go",
		`{"type":"assistant","parent_tool_use_id":"t1","message":{"content":[{"type":"tool_use","name":"Grep","input":{"pattern":"Foo"}}]}}`: "（サブエージェント）Grep: Foo",
		`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":""}]}}`:                                                     "",
	}
	for line, want := range tests {
		var e streamEvent
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatal(err)
		}
		if got := e.activity(); got != want {
			t.Errorf("activity(%s) = %q, want %q", line, got, want)
		}
	}
}

func receive(t *testing.T, ch <-chan usecase.ProcessEvent) usecase.ProcessEvent {
	t.Helper()
	select {
	case e, ok := <-ch:
		if !ok {
			t.Fatal("Events closed")
		}
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
		return usecase.ProcessEvent{}
	}
}

// fakeAPI is a container whose stdin and multiplexed output are pipes the test controls.
type fakeAPI struct {
	mu      sync.Mutex
	started bool
	removed bool
	status  chan int64

	stdin      chan []byte
	output     *io.PipeWriter
	outputRead *io.PipeReader
}

func newFakeAPI() *fakeAPI {
	r, w := io.Pipe()
	return &fakeAPI{status: make(chan int64, 1), stdin: make(chan []byte, 8), output: w, outputRead: r}
}

func (f *fakeAPI) ContainerCreate(context.Context, client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	return client.ContainerCreateResult{ID: "c1"}, nil
}

func (f *fakeAPI) ContainerAttach(context.Context, string, client.ContainerAttachOptions) (client.ContainerAttachResult, error) {
	local, remote := net.Pipe()
	go func() {
		r := bufio.NewReader(remote)
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			f.stdin <- line
		}
	}()
	return client.ContainerAttachResult{HijackedResponse: client.HijackedResponse{Conn: local, Reader: bufio.NewReader(f.outputRead)}}, nil
}

func (f *fakeAPI) ContainerStart(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = true
	return client.ContainerStartResult{}, nil
}

func (f *fakeAPI) ContainerWait(context.Context, string, client.ContainerWaitOptions) client.ContainerWaitResult {
	res := make(chan container.WaitResponse, 1)
	res <- container.WaitResponse{StatusCode: <-f.status}
	return client.ContainerWaitResult{Result: res, Error: make(chan error)}
}

func (f *fakeAPI) ContainerRemove(context.Context, string, client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = true
	return client.ContainerRemoveResult{}, nil
}

func (f *fakeAPI) readStdin(t *testing.T) []byte {
	t.Helper()
	select {
	case line := <-f.stdin:
		return line
	case <-time.After(2 * time.Second):
		t.Fatal("nothing was written to stdin")
		return nil
	}
}

// emit writes data to the container's stdout (1) or stderr (2) in Docker's non-TTY framing.
func (f *fakeAPI) emit(stream byte, data string) {
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
	_, _ = f.output.Write(append(header, data...))
}

func (f *fakeAPI) exit(status int64) {
	f.status <- status
	_ = f.output.Close()
}
