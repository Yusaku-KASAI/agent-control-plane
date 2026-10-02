package usecase_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

type fakeRegistry map[domain.AgentName]domain.Agent

func (f fakeRegistry) FindByName(_ context.Context, name domain.AgentName) (domain.Agent, error) {
	a, ok := f[name]
	if !ok {
		return domain.Agent{}, domain.ErrAgentNotFound
	}
	return a, nil
}

type fakeSessions struct {
	mu    sync.Mutex
	saved map[domain.SessionID]domain.Session
	busy  bool
}

func (f *fakeSessions) Find(_ context.Context, id domain.SessionID) (domain.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.saved[id]
	if !ok {
		return domain.Session{}, domain.ErrSessionNotFound
	}
	return s, nil
}

func (f *fakeSessions) Save(_ context.Context, s domain.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved[s.ID] = s
	return nil
}

func (f *fakeSessions) Lock(context.Context, domain.SessionID) (func(), error) {
	if f.busy {
		return nil, domain.ErrSessionBusy
	}
	return func() {}, nil
}

type fakeWorkspaces struct {
	err      error
	prepared int
}

func (f *fakeWorkspaces) Prepare(_ context.Context, _ domain.Agent, id domain.SessionID, _ domain.Credentials) (domain.Workspace, error) {
	f.prepared++
	return domain.Workspace{Path: "/sessions/" + string(id)}, f.err
}

type fakeIssuer struct {
	mu     sync.Mutex
	issued int
	err    error
}

func (f *fakeIssuer) Issue(context.Context, domain.Agent) (domain.Credentials, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issued++
	return domain.Credentials{GitHubToken: "token"}, f.err
}

// fakeProcess answers each prompt when the test calls finish, mimicking a turn that runs until done.
type fakeProcess struct {
	mu          sync.Mutex
	prompts     []string
	interrupted bool
	updated     int
	closed      bool
	events      chan usecase.ProcessEvent
	err         error
}

func (p *fakeProcess) Send(_ context.Context, prompt string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prompts = append(p.prompts, prompt)
	return nil
}

func (p *fakeProcess) Interrupt(context.Context) error {
	p.mu.Lock()
	p.interrupted = true
	p.mu.Unlock()
	p.events <- usecase.ProcessEvent{Kind: usecase.EventTurnFinished, Result: usecase.TurnResult{Interrupted: true}}
	return nil
}

func (p *fakeProcess) Events() <-chan usecase.ProcessEvent { return p.events }
func (p *fakeProcess) Err() error                          { return p.err }

func (p *fakeProcess) UpdateCredentials(context.Context, domain.Credentials) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.updated++
	return nil
}

func (p *fakeProcess) Close(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

func (p *fakeProcess) sent() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.prompts...)
}

func (p *fakeProcess) finish(output string) {
	p.events <- usecase.ProcessEvent{Kind: usecase.EventTurnFinished, Result: usecase.TurnResult{Output: output}}
}

func (p *fakeProcess) startTurn() { p.events <- usecase.ProcessEvent{Kind: usecase.EventTurnStarted} }

func (p *fakeProcess) background(n int) {
	p.events <- usecase.ProcessEvent{Kind: usecase.EventBackgroundTasks, BackgroundTasks: n}
}

func (p *fakeProcess) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

type fakeListener struct {
	mu    sync.Mutex
	turns []usecase.TurnResult
}

func (f *fakeListener) AgentTurn(_ context.Context, _ domain.Session, r usecase.TurnResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.turns = append(f.turns, r)
}

func (f *fakeListener) received() []usecase.TurnResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]usecase.TurnResult(nil), f.turns...)
}

type fakeRunner struct {
	mu      sync.Mutex
	started []usecase.StartRequest
	procs   chan *fakeProcess
}

func newFakeRunner() *fakeRunner { return &fakeRunner{procs: make(chan *fakeProcess, 8)} }

func (f *fakeRunner) Start(_ context.Context, r usecase.StartRequest) (usecase.Process, error) {
	p := &fakeProcess{events: make(chan usecase.ProcessEvent, 8)}
	f.mu.Lock()
	f.started = append(f.started, r)
	f.mu.Unlock()
	f.procs <- p
	return p, nil
}

func (f *fakeRunner) starts() []usecase.StartRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]usecase.StartRequest(nil), f.started...)
}

func (f *fakeRunner) next(t *testing.T) *fakeProcess {
	t.Helper()
	select {
	case p := <-f.procs:
		return p
	case <-time.After(time.Second):
		t.Fatal("no process was started")
		return nil
	}
}

var (
	dev = domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: "/src/repo", Image: "img"}}
	pm  = domain.Agent{Name: "repo-pm", Role: "pm", Target: domain.Target{Path: "/src/repo", Image: "img"}}
)

const existingID domain.SessionID = "0f8fad5b-d9cb-469f-a165-70867728950e"

type env struct {
	sessions   *fakeSessions
	workspaces *fakeWorkspaces
	issuer     *fakeIssuer
	runner     *fakeRunner
	listener   *fakeListener
	d          *usecase.Dispatcher
}

func newEnv(t *testing.T, opts usecase.DispatcherOptions) env {
	t.Helper()
	if opts.IdleTimeout == 0 {
		opts.IdleTimeout = time.Hour
	}
	if opts.CredentialRefresh == 0 {
		opts.CredentialRefresh = time.Hour
	}
	opts.StopTimeout = time.Second
	e := env{
		sessions: &fakeSessions{saved: map[domain.SessionID]domain.Session{
			existingID: {ID: existingID, AgentName: dev.Name, Workspace: domain.Workspace{Path: "/sessions/x"}, Turns: 1},
		}},
		workspaces: &fakeWorkspaces{},
		issuer:     &fakeIssuer{},
		runner:     newFakeRunner(),
		listener:   &fakeListener{},
	}
	e.d = usecase.NewDispatcher(fakeRegistry{dev.Name: dev, pm.Name: pm}, e.sessions, e.workspaces, e.issuer, e.runner, e.listener, opts)
	t.Cleanup(e.d.Close)
	return e
}

type outcome struct {
	out usecase.RunAgentOutput
	err error
}

func (e env) execute(in usecase.RunAgentInput) <-chan outcome {
	ch := make(chan outcome, 1)
	go func() {
		out, err := e.d.Execute(context.Background(), in)
		ch <- outcome{out, err}
	}()
	return ch
}

func wait(t *testing.T, ch <-chan outcome) outcome {
	t.Helper()
	select {
	case o := <-ch:
		return o
	case <-time.After(2 * time.Second):
		t.Fatal("Execute did not return")
		return outcome{}
	}
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition was not met")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDispatcher_NewSession(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	thread := domain.Conversation{Platform: "slack", Workspace: "T1", Channel: "C1", Thread: "1.0"}
	done := e.execute(usecase.RunAgentInput{AgentName: dev.Name, Prompt: "fix the bug", Conversation: thread})

	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("done")

	o := wait(t, done)
	if o.err != nil || o.out.Output != "done" || o.out.Session.Turns != 1 || o.out.Session.Conversation != thread {
		t.Fatalf("Execute() = %+v, %v", o.out, o.err)
	}
	if start := e.runner.starts()[0]; !start.Session.IsNew() || start.Credentials.GitHubToken != "token" {
		t.Errorf("process started with %+v, want a new session with credentials", start)
	}
	if e.workspaces.prepared != 1 {
		t.Errorf("workspaces prepared = %d, want 1", e.workspaces.prepared)
	}
}

func TestDispatcher_QueuesBehindRunningTurn(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "first"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })

	second := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "second"})
	time.Sleep(20 * time.Millisecond)
	if got := p.sent(); len(got) != 1 {
		t.Fatalf("sent %q while the first turn was running, want only the first prompt", got)
	}

	p.finish("one")
	if o := wait(t, first); o.err != nil || o.out.Output != "one" {
		t.Fatalf("first = %+v, %v", o.out, o.err)
	}
	eventually(t, func() bool { return len(p.sent()) == 2 })
	p.finish("two")
	if o := wait(t, second); o.err != nil || o.out.Output != "two" || o.out.Session.Turns != 3 {
		t.Fatalf("second = %+v, %v", o.out, o.err)
	}
	if n := len(e.runner.starts()); n != 1 {
		t.Errorf("started %d processes, want 1", n)
	}
	if e.runner.starts()[0].Session.IsNew() {
		t.Error("an existing session was started as new instead of resumed")
	}
}

func TestDispatcher_InjectsIntoRunningTurn(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "first"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })

	steer := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "also this", Inject: true})
	eventually(t, func() bool { return len(p.sent()) == 2 })
	p.finish("both")

	for name, ch := range map[string]<-chan outcome{"first": first, "injected": steer} {
		if o := wait(t, ch); o.err != nil || o.out.Output != "both" {
			t.Errorf("%s = %+v, %v; want the shared turn's output", name, o.out, o.err)
		}
	}
}

func TestDispatcher_Interrupt(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	running := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "long task"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })

	if err := e.d.Interrupt(context.Background(), dev.Name, existingID); err != nil {
		t.Fatalf("Interrupt() error = %v", err)
	}
	if o := wait(t, running); o.err != nil || !o.out.Interrupted {
		t.Errorf("interrupted turn = %+v, %v", o.out, o.err)
	}
	if err := e.d.Interrupt(context.Background(), pm.Name, existingID); !errors.Is(err, domain.ErrSessionAgentMismatch) {
		t.Errorf("Interrupt(other agent) error = %v, want ErrSessionAgentMismatch", err)
	}
}

func TestDispatcher_ClosesIdleProcessAndResumes(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{IdleTimeout: 30 * time.Millisecond})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "first"})
	p1 := e.runner.next(t)
	eventually(t, func() bool { return len(p1.sent()) == 1 })
	p1.finish("one")
	wait(t, first)

	eventually(t, p1.isClosed)

	second := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "second"})
	p2 := e.runner.next(t)
	eventually(t, func() bool { return len(p2.sent()) == 1 })
	p2.finish("two")
	if o := wait(t, second); o.err != nil || o.out.Output != "two" {
		t.Fatalf("second = %+v, %v", o.out, o.err)
	}
	if starts := e.runner.starts(); starts[1].Session.Turns != 2 {
		t.Errorf("restarted with turns = %d, want 2 so that it resumes", starts[1].Session.Turns)
	}
}

func TestDispatcher_RefreshesCredentials(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{CredentialRefresh: 20 * time.Millisecond})
	running := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "long task"})
	p := e.runner.next(t)
	eventually(t, func() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.updated >= 2 })
	p.finish("done")
	wait(t, running)
}

func TestDispatcher_ProcessExitFailsRunningTurn(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	running := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "crash"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })

	p.err = errors.New("exit status 1")
	close(p.events)
	if o := wait(t, running); o.err == nil || o.out.Session.ID != existingID {
		t.Fatalf("Execute() = %+v, %v; want an error with the session", o.out, o.err)
	}
}

func TestDispatcher_Errors(t *testing.T) {
	tests := []struct {
		name    string
		in      usecase.RunAgentInput
		setup   func(env)
		wantErr error
	}{
		{name: "empty prompt", in: usecase.RunAgentInput{AgentName: dev.Name, Prompt: " "}, wantErr: usecase.ErrEmptyPrompt},
		{name: "unknown agent", in: usecase.RunAgentInput{AgentName: "nobody", Prompt: "hi"}, wantErr: domain.ErrAgentNotFound},
		{
			name:    "unknown session",
			in:      usecase.RunAgentInput{AgentName: dev.Name, SessionID: "11111111-1111-4111-8111-111111111111", Prompt: "hi"},
			wantErr: domain.ErrSessionNotFound,
		},
		{
			name:    "session of another agent",
			in:      usecase.RunAgentInput{AgentName: pm.Name, SessionID: existingID, Prompt: "hi"},
			wantErr: domain.ErrSessionAgentMismatch,
		},
		{
			name:    "session held by another control plane",
			in:      usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "hi"},
			setup:   func(e env) { e.sessions.busy = true },
			wantErr: domain.ErrSessionBusy,
		},
		{
			name:    "workspace failure",
			in:      usecase.RunAgentInput{AgentName: dev.Name, Prompt: "hi"},
			setup:   func(e env) { e.workspaces.err = errors.New("clone failed") },
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, usecase.DispatcherOptions{})
			if tt.setup != nil {
				tt.setup(e)
			}
			_, err := e.d.Execute(context.Background(), tt.in)
			if err == nil || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
				t.Fatalf("Execute() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDispatcher_CreateIfMissing(t *testing.T) {
	const id domain.SessionID = "22222222-2222-5222-8222-222222222222"
	e := newEnv(t, usecase.DispatcherOptions{})
	in := usecase.RunAgentInput{AgentName: dev.Name, SessionID: id, CreateIfMissing: true, Prompt: "hi"}

	first := e.execute(in)
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("one")
	if o := wait(t, first); o.err != nil || o.out.Session.ID != id {
		t.Fatalf("first = %+v, %v", o.out, o.err)
	}

	second := e.execute(in)
	eventually(t, func() bool { return len(p.sent()) == 2 })
	p.finish("two")
	if o := wait(t, second); o.err != nil || e.workspaces.prepared != 1 {
		t.Fatalf("second = %+v, %v (prepared %d), want the same session", o.out, o.err, e.workspaces.prepared)
	}
}

func TestDispatcher_AgentStartedTurn(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "start a build"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("building in the background")
	wait(t, first)

	// The agent resumes by itself when the background work ends; a prompt arriving meanwhile waits.
	p.startTurn()
	queued := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "next"})
	time.Sleep(20 * time.Millisecond)
	if got := p.sent(); len(got) != 1 {
		t.Fatalf("sent %q during the agent's own turn, want it queued", got)
	}
	p.finish("build passed")
	eventually(t, func() bool { return len(e.listener.received()) == 1 })
	if got := e.listener.received()[0].Output; got != "build passed" {
		t.Errorf("listener got %q", got)
	}

	eventually(t, func() bool { return len(p.sent()) == 2 })
	p.finish("ok")
	if o := wait(t, queued); o.err != nil || o.out.Output != "ok" {
		t.Errorf("queued = %+v, %v", o.out, o.err)
	}
}

func TestDispatcher_KeepsProcessWhileBackgroundTasksRun(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{IdleTimeout: 30 * time.Millisecond})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "start a build"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.background(1)
	p.finish("building")
	wait(t, first)

	time.Sleep(100 * time.Millisecond)
	if p.isClosed() {
		t.Fatal("the process was closed while a background task was running")
	}
	p.background(0)
	eventually(t, p.isClosed)
}
