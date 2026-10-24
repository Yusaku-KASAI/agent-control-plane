package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"maps"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
		[]pluginDir{
			{Path: "platform/plugins/roles"},
			{Path: "/workspace/kit/platform/plugins/slack", Beside: &mountedDir{Host: "/src/kit/platform", Container: "/workspace/kit/platform"}},
			{Path: "/workspace/kit/platform/plugins/github", Beside: &mountedDir{Host: "/src/kit/platform", Container: "/workspace/kit/platform"}},
		}, Options{
			Tools:           ToolsOptions{Dir: "/var/lib/acp/tools", Volume: "acp-tools"},
			ClaudeConfigDir: "/var/lib/acp/claude", ClaudeConfigVolume: "acp-claude",
			CredentialsDir: "/var/lib/acp/credentials", CredentialsVolume: "acp-credentials",
		})

	if spec.Config.Image != "repo-dev:latest" || spec.Config.WorkingDir != "/workspace/repo" {
		t.Errorf("Image = %q, WorkingDir = %q", spec.Config.Image, spec.Config.WorkingDir)
	}
	if !spec.Config.OpenStdin || !spec.Config.StdinOnce || !spec.Config.AttachStdin {
		t.Error("stdin is not open for prompts")
	}
	wantCmd := []string{
		"claude", "-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--replay-user-messages",
		"--verbose",
		"--agent", "developer",
		"--session-id", "0f8fad5b-d9cb-469f-a165-70867728950e",
		"--dangerously-skip-permissions",
		"--plugin-dir", "/workspace/repo/platform/plugins/roles",
		"--plugin-dir", "/workspace/kit/platform/plugins/slack",
		"--plugin-dir", "/workspace/kit/platform/plugins/github",
	}
	if !slices.Equal(spec.Config.Cmd, wantCmd) {
		t.Errorf("Cmd = %q, want %q", spec.Config.Cmd, wantCmd)
	}
	for _, want := range []string{"ACP_SESSION_BRANCH=acp/0f8fad5b", "ACP_WORKSPACE=/workspace/repo", "ACP_CO_AUTHORS_FILE=/run/acp/credentials/git/co-authors"} {
		if !slices.Contains(spec.Config.Env, want) {
			t.Errorf("Env = %q, want %s", spec.Config.Env, want)
		}
	}
	wantLabels := map[string]string{
		"acp.session": string(testSession.ID),
		"acp.agent":   "repo-dev",
		// Blank, so that the labels the image got from docker compose build are not inherited.
		"com.docker.compose.project": "",
		"com.docker.compose.service": "",
	}
	if !maps.Equal(spec.Config.Labels, wantLabels) {
		t.Errorf("Labels = %v, want %v", spec.Config.Labels, wantLabels)
	}
	wantMounts := []mount.Mount{
		{Type: mount.TypeBind, Source: "/sessions/0f8fad5b/workspace/repo", Target: "/workspace/repo"},
		{
			Type: mount.TypeVolume, Source: "acp-claude", Target: "/home/node/.claude",
			VolumeOptions: &mount.VolumeOptions{Subpath: "repo-dev"},
		},
		{
			Type: mount.TypeVolume, Source: "acp-credentials", Target: "/run/acp/credentials", ReadOnly: true,
			VolumeOptions: &mount.VolumeOptions{Subpath: "0f8fad5b-d9cb-469f-a165-70867728950e"},
		},
		// For the agent to run acp cleanup-check (the harness contract).
		{Type: mount.TypeVolume, Source: "acp-tools", Target: "/run/acp/bin", ReadOnly: true},
		{Type: mount.TypeBind, Source: "/src/kit/platform", Target: "/workspace/kit/platform", ReadOnly: true},
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

func TestContainerSpec_BindMountsWithoutVolumes(t *testing.T) {
	mounts := containerSpec(usecase.StartRequest{Agent: testAgent, Session: testSession}, nil,
		Options{CredentialsDir: "/host/creds", ClaudeConfigDir: "/host/claude", Tools: ToolsOptions{Dir: "/host/tools"}}).HostConfig.Mounts
	if m := mounts[1]; m.Type != mount.TypeBind || m.Source != "/host/claude/repo-dev" || m.Target != "/home/node/.claude" {
		t.Errorf("claude config mount = %+v", m)
	}
	if m := mounts[2]; m.Type != mount.TypeBind || m.Source != "/host/creds/0f8fad5b-d9cb-469f-a165-70867728950e" || !m.ReadOnly {
		t.Errorf("credentials mount = %+v", m)
	}
	if m := mounts[3]; m.Type != mount.TypeBind || m.Source != "/host/tools" || m.Target != "/run/acp/bin" || !m.ReadOnly {
		t.Errorf("tools mount = %+v", m)
	}
}

func TestContainerSpec_ClaudeAPIKey(t *testing.T) {
	env := containerSpec(usecase.StartRequest{Agent: testAgent, Session: testSession, Credentials: domain.Credentials{ClaudeAPIKey: "sk-ant-api-x"}}, nil, Options{}).Config.Env
	if !slices.Contains(env, "ANTHROPIC_API_KEY=sk-ant-api-x") {
		t.Errorf("Env = %q, want the API key", env)
	}
}

func TestContainerSpec_Credentials(t *testing.T) {
	agent := testAgent
	agent.Git = domain.GitIdentity{Name: "repo-dev (agent)", Email: "1+u@users.noreply.github.com"}
	req := usecase.StartRequest{Agent: agent, Session: testSession, Credentials: domain.Credentials{
		GitHubToken: "ghs_x", SlackBotToken: "xoxb-x", ClaudeOAuthToken: "sk-ant-oat-x",
	}}

	env := containerSpec(req, nil, Options{}).Config.Env
	for _, want := range []string{
		"GIT_AUTHOR_NAME=repo-dev (agent)",
		"GIT_COMMITTER_EMAIL=1+u@users.noreply.github.com",
		"CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat-x",
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
		if strings.HasPrefix(e, "GH_") || strings.HasPrefix(e, "GIT_") || strings.HasPrefix(e, "SLACK_") || strings.HasPrefix(e, "CLAUDE_CODE_OAUTH") {
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
		b, err = os.ReadFile(filepath.Join(c.dir(), "gh", "config.yml"))
		if err != nil || string(b) != "version: \"1\"\n" {
			t.Errorf("config.yml = %q, %v; want version 1 so gh skips migration", b, err)
		}
		b, err = os.ReadFile(filepath.Join(c.dir(), "slack", "bot-token"))
		if err != nil || string(b) != "xoxb-"+n {
			t.Errorf("slack bot-token = %q, %v; want xoxb-%s", b, err, n)
		}
	}
}

// The container mounts the credentials read-only, and gh must still hand git the token.
func TestSessionCredentials_Write_ReadOnlyGh(t *testing.T) {
	gh, err := exec.LookPath("gh")
	if err != nil {
		t.Skip("gh not installed")
	}
	c := sessionCredentials{root: t.TempDir(), id: testSession.ID}
	if err := c.write(domain.Credentials{GitHubToken: "ghs_token"}); err != nil {
		t.Fatalf("write() error = %v", err)
	}
	dir := filepath.Join(c.dir(), "gh")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	cmd := exec.Command(gh, "auth", "git-credential", "get")
	cmd.Env = append(os.Environ(), "GH_CONFIG_DIR="+dir, "GH_NO_UPDATE_NOTIFIER=1")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "password=ghs_token") {
		t.Errorf("gh auth git-credential get = %q, %v; want the token", out, err)
	}
}

func TestSessionCredentials_WriteCoAuthors(t *testing.T) {
	c := sessionCredentials{root: t.TempDir(), id: testSession.ID}
	if err := c.write(domain.Credentials{}); err != nil {
		t.Fatal(err)
	}
	people := []domain.GitIdentity{{Name: "Alice", Email: "alice@example.com"}, {Name: "Bob", Email: "bob@example.com"}}
	if err := c.writeCoAuthors(people); err != nil {
		t.Fatalf("writeCoAuthors() error = %v", err)
	}
	b, err := os.ReadFile(filepath.Join(c.dir(), "git", "co-authors"))
	if want := "Alice <alice@example.com>\nBob <bob@example.com>\n"; err != nil || string(b) != want {
		t.Errorf("co-authors = %q, %v; want %q", b, err, want)
	}
}

func TestRunner_Start(t *testing.T) {
	api := newFakeAPI()
	runner := NewRunner(api, Options{CredentialsDir: t.TempDir(), ClaudeConfigDir: t.TempDir()})
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
	api.emit(1, `{"type":"result","subtype":"success","result":"done","total_cost_usd":0.75,"usage":{"input_tokens":9},`+
		`"modelUsage":{"claude-opus":{"inputTokens":1,"outputTokens":2,"cacheReadInputTokens":3,"cacheCreationInputTokens":4,"webSearchRequests":0,"costUSD":0.5,"contextWindow":200000},`+
		`"claude-haiku":{"inputTokens":5,"outputTokens":6,"cacheReadInputTokens":0,"cacheCreationInputTokens":0,"costUSD":0.25}}}`+"\n")
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
	finished := receive(t, proc.Events())
	if finished.Kind != usecase.EventTurnFinished || finished.Result.Output != "done" || finished.Result.Interrupted {
		t.Errorf("event = %+v, want the result", finished)
	}
	wantUsage := domain.Usage{CostUSD: 0.75, Models: map[string]domain.ModelUsage{
		"claude-opus":  {InputTokens: 1, OutputTokens: 2, CacheReadInputTokens: 3, CacheCreationInputTokens: 4, CostUSD: 0.5},
		"claude-haiku": {InputTokens: 5, OutputTokens: 6, CostUSD: 0.25},
	}}
	if !reflect.DeepEqual(finished.Result.Usage, wantUsage) {
		t.Errorf("usage = %+v, want %+v", finished.Result.Usage, wantUsage)
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
	if e := receive(t, proc.Events()); !e.Result.Interrupted || e.Result.Error {
		t.Errorf("event = %+v, want an interrupted result", e)
	}
	stops := []struct {
		name      string
		interrupt bool
		lines     []string
		stopped   bool
		output    string
	}{
		{"aborted without an interrupt", false, []string{`{"type":"result","subtype":"error_during_execution","terminal_reason":"aborted_tools","errors":["turn_handoff"]}`}, true, ""},
		{"aborted while streaming", false, []string{`{"type":"result","subtype":"error_during_execution","terminal_reason":"aborted_streaming"}`}, true, ""},
		{"failed setup", false, []string{`{"type":"result","subtype":"error_during_execution","terminal_reason":"turn_setup_failed","errors":["queryParams builder failed: boom"]}`}, false, "queryParams builder failed: boom"},
		{"exception without a reason", false, []string{`{"type":"result","subtype":"error_during_execution","errors":["a","b"]}`}, false, "a\nb"},
		{"exception with diagnostics", false, []string{`{"type":"result","subtype":"error_during_execution","errors":["[ede_diagnostic] result_type=assistant","boom","[session_crash] error_name=Error"]}`}, false, "boom"},
		{"interrupt taken by a finished turn", true, []string{`{"type":"result","subtype":"success","result":"done"}`,
			`{"type":"result","subtype":"error_during_execution","errors":["boom"]}`}, false, "boom"},
	}
	for _, s := range stops {
		if s.interrupt {
			if err := proc.Interrupt(context.Background()); err != nil {
				t.Fatalf("Interrupt() error = %v", err)
			}
			api.readStdin(t)
		}
		var e usecase.ProcessEvent
		for _, line := range s.lines {
			api.emit(1, line+"\n")
			e = receive(t, proc.Events())
		}
		if e.Result.Interrupted != s.stopped || e.Result.Error == s.stopped || e.Result.Output != s.output {
			t.Errorf("%s: event = %+v, want stopped %v with output %q", s.name, e, s.stopped, s.output)
		}
	}

	api.emit(1, `{"type":"result","subtype":"success","is_error":true,"result":"API Error: 500"}`+"\n")
	if e := receive(t, proc.Events()); !e.Result.Error || e.Result.UsageLimited || e.Result.Output != "API Error: 500" {
		t.Errorf("event = %+v, want an error result", e)
	}
	results := []struct {
		name  string
		lines []string
		want  bool
	}{
		{"typed as a usage limit", []string{`{"type":"result","subtype":"success","is_error":true,"api_error":"usage_limit_reached","result":"You've hit your session limit"}`}, true},
		{"refused by the limit", []string{`{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","resetsAt":4102444800,"rateLimitType":"five_hour"}}`,
			`{"type":"result","subtype":"success","is_error":true,"result":"You've hit your session limit"}`}, true},
		{"typed as another error while refused", []string{`{"type":"result","subtype":"success","is_error":true,"api_error":"no_response","result":"API Error"}`}, false},
		{"over the limit on overage", []string{`{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","isUsingOverage":true,"resetsAt":4102444800}}`,
			`{"type":"result","subtype":"success","is_error":true,"result":"API Error: 500"}`}, false},
		{"refusal past its reset", []string{`{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","resetsAt":1700000000}}`,
			`{"type":"result","subtype":"success","is_error":true,"result":"API Error: 500"}`}, false},
		{"allowed again", []string{`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed"}}`,
			`{"type":"result","subtype":"error_max_turns","is_error":true,"result":""}`}, false},
	}
	for _, r := range results {
		for _, line := range r.lines {
			api.emit(1, line+"\n")
		}
		if e := receive(t, proc.Events()); !e.Result.Error || e.Result.UsageLimited != r.want {
			t.Errorf("%s: event = %+v, want an error with usage limited %v", r.name, e, r.want)
		}
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
	tests := []struct {
		line             string
		activity, remark string
	}{
		{`{"type":"assistant","message":{"content":[{"type":"text","text":"Found the cause.\n\n  Fixing it next.\n"}]}}`, "", "Found the cause.\n\n  Fixing it next."},
		{`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"make check"}}]}}`, "Bash: make check", ""},
		{`{"type":"assistant","message":{"content":[{"type":"text","text":"Running the tests."},{"type":"tool_use","name":"Edit","input":{"file_path":"/w/a.go"}}]}}`, "Edit: /w/a.go", "Running the tests."},
		{`{"type":"assistant","message":{"content":[{"type":"text","text":"One."},{"type":"text","text":"Two."}]}}`, "", "One.\n\nTwo."},
		{`{"type":"assistant","parent_tool_use_id":"t1","message":{"content":[{"type":"tool_use","name":"Grep","input":{"pattern":"Foo"}}]}}`, "（サブエージェント）Grep: Foo", ""},
		{`{"type":"assistant","parent_tool_use_id":"t1","message":{"content":[{"type":"text","text":"Looking at the tests\nmore"}]}}`, "（サブエージェント）Looking at the tests", ""},
		{`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":""},{"type":"text","text":"  "}]}}`, "", ""},
	}
	for _, tc := range tests {
		var e streamEvent
		if err := json.Unmarshal([]byte(tc.line), &e); err != nil {
			t.Fatal(err)
		}
		pe, ok := e.processEvent(turnFacts{})
		if want := tc.activity != "" || tc.remark != ""; ok != want {
			t.Errorf("processEvent(%s) reported %v, want %v", tc.line, ok, want)
		}
		if ok && (pe.Kind != usecase.EventActivity || pe.Activity != tc.activity || pe.Remark != tc.remark) {
			t.Errorf("processEvent(%s) = %+v, want activity %q and remark %q", tc.line, pe, tc.activity, tc.remark)
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

func TestRunner_RemoveLeftovers(t *testing.T) {
	api := newFakeAPI()
	api.containers = []container.Summary{
		{ID: "a", Labels: map[string]string{"acp.session": "s1"}},
		{ID: "b", Labels: map[string]string{"acp.session": "s2"}},
		{ID: "c", Labels: map[string]string{"acp.session": "s1"}},
		{ID: "r", Labels: map[string]string{"acp.workspace-state": ""}},
	}

	ids, err := NewRunner(api, Options{}).RemoveLeftovers(context.Background())

	if err != nil || !slices.Equal(ids, []domain.SessionID{"s1", "s2"}) {
		t.Errorf("RemoveLeftovers() = %v, %v", ids, err)
	}
	slices.Sort(api.removedIDs)
	if !slices.Equal(api.removedIDs, []string{"a", "b", "c", "r"}) {
		t.Errorf("removed %v, want every agent container and workspace reader", api.removedIDs)
	}
	for _, label := range []string{"acp.session", "acp.workspace-state"} {
		if !slices.ContainsFunc(api.listed, func(o client.ContainerListOptions) bool { return o.All && o.Filters["label"][label] }) {
			t.Errorf("listed with %+v, want stopped ones too, by the label %s", api.listed, label)
		}
	}
}

// fakeAPI is a container whose stdin and multiplexed output are pipes the test controls.
type fakeAPI struct {
	mu         sync.Mutex
	started    bool
	removed    bool
	removedIDs []string
	status     chan int64
	// containers is what ContainerList returns.
	containers []container.Summary
	listed     []client.ContainerListOptions

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

func (f *fakeAPI) ContainerRemove(_ context.Context, id string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = true
	f.removedIDs = append(f.removedIDs, id)
	return client.ContainerRemoveResult{}, nil
}

func (f *fakeAPI) ContainerList(_ context.Context, opts client.ContainerListOptions) (client.ContainerListResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listed = append(f.listed, opts)
	var items []container.Summary
	for _, c := range f.containers {
		for label := range opts.Filters["label"] {
			if _, ok := c.Labels[label]; ok {
				items = append(items, c)
			}
		}
	}
	return client.ContainerListResult{Items: items}, nil
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

// Agents rely on the harness contract, not on this code, for what the container is given.
func TestContainerSpec_EnvInHarnessContract(t *testing.T) {
	contract, err := os.ReadFile("../../../platform/docs/reference/harness.md")
	if err != nil {
		t.Fatal(err)
	}
	agent := testAgent
	agent.Git = domain.GitIdentity{Name: "repo-dev (agent)", Email: "1+u@users.noreply.github.com"}
	for _, creds := range []domain.Credentials{
		{GitHubToken: "ghs_x", SlackBotToken: "xoxb-x", ClaudeOAuthToken: "sk-ant-oat-x"},
		{ClaudeAPIKey: "sk-ant-api-x"},
	} {
		env := containerSpec(usecase.StartRequest{Agent: agent, Session: testSession, Credentials: creds}, nil, Options{}).Config.Env
		for _, kv := range env {
			name, value, _ := strings.Cut(kv, "=")
			// How git and gh are configured is not part of the contract.
			if strings.HasPrefix(name, "GIT_CONFIG_") || name == "GH_NO_UPDATE_NOTIFIER" {
				continue
			}
			// The contract names the workspace by the target's directory, which differs per agent.
			if name == "ACP_WORKSPACE" {
				value = "/workspace/<名前>"
			}
			if !strings.Contains(string(contract), "`"+name+"`") {
				t.Errorf("harness.md does not describe %s", name)
			}
			if strings.HasPrefix(value, "/") && !strings.Contains(string(contract), value) {
				t.Errorf("harness.md does not give the path of %s (%s)", name, value)
			}
		}
	}
}

func TestProcess_CloseStopsProcessThatDoesNotExit(t *testing.T) {
	api := newFakeAPI()
	runner := NewRunner(api, Options{CredentialsDir: t.TempDir(), ClaudeConfigDir: t.TempDir()})
	proc, err := runner.Start(context.Background(), usecase.StartRequest{Agent: testAgent, Session: testSession})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := proc.Close(ctx); err == nil || !api.removed {
		t.Errorf("Close() error = %v, removed = %v; want an error and the container removed", err, api.removed)
	}
}
