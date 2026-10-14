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
	// orphans are stored sessions without a record.
	orphans map[domain.SessionID]bool
	// held are sessions locked by someone outside the test; locks counts the locks the test holds.
	held    map[domain.SessionID]bool
	locks   map[domain.SessionID]int
	removed []domain.SessionID
	saves   int
	// onLock runs before Lock takes the lock.
	onLock func(id domain.SessionID)
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
	f.saves++
	return nil
}

func (f *fakeSessions) Lock(_ context.Context, id domain.SessionID) (func(), error) {
	if f.onLock != nil {
		f.onLock(id)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.busy || f.held[id] {
		return nil, domain.ErrSessionBusy
	}
	if f.locks == nil {
		f.locks = make(map[domain.SessionID]int)
	}
	f.locks[id]++
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.locks[id]--
	}, nil
}

func (f *fakeSessions) List(context.Context) ([]domain.SessionID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []domain.SessionID
	for id := range f.saved {
		ids = append(ids, id)
	}
	for id := range f.orphans {
		ids = append(ids, id)
	}
	return ids, nil
}

func (f *fakeSessions) isLocked(id domain.SessionID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.locks[id] > 0
}

func (f *fakeSessions) Remove(_ context.Context, id domain.SessionID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.locks[id] == 0 {
		return errors.New("removed without the lock")
	}
	delete(f.saved, id)
	delete(f.orphans, id)
	f.removed = append(f.removed, id)
	return nil
}

type fakeWorkspaces struct {
	err      error
	prepared int
	sessions *fakeSessions
}

func (f *fakeWorkspaces) Prepare(_ context.Context, _ domain.Agent, id domain.SessionID, _ domain.Credentials) (domain.Workspace, error) {
	if !f.sessions.isLocked(id) {
		return domain.Workspace{}, errors.New("prepared without the lock")
	}
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
	mu      sync.Mutex
	prompts []string
	ids     []string
	// holdPrompts keeps prompts from being taken in until accept is called.
	holdPrompts bool
	interrupted bool
	stoppedTask []string
	updated     int
	coAuthors   []domain.GitIdentity
	closed      bool
	events      chan usecase.ProcessEvent
	err         error
}

// Send records the prompt last, so a test that has seen it may close the events channel.
func (p *fakeProcess) Send(_ context.Context, id, prompt string) error {
	p.mu.Lock()
	hold := p.holdPrompts
	p.mu.Unlock()
	if !hold {
		p.events <- usecase.ProcessEvent{Kind: usecase.EventPromptAccepted, PromptID: id}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prompts = append(p.prompts, prompt)
	p.ids = append(p.ids, id)
	return nil
}

// accept reports that the agent took the i-th prompt into its turn.
func (p *fakeProcess) accept(i int) {
	p.mu.Lock()
	id := p.ids[i]
	p.mu.Unlock()
	p.events <- usecase.ProcessEvent{Kind: usecase.EventPromptAccepted, PromptID: id}
}

func (p *fakeProcess) hold() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.holdPrompts = true
}

func (p *fakeProcess) Interrupt(context.Context) error {
	p.mu.Lock()
	p.interrupted = true
	p.mu.Unlock()
	p.events <- usecase.ProcessEvent{Kind: usecase.EventTurnFinished, Result: usecase.TurnResult{Interrupted: true}}
	return nil
}

func (p *fakeProcess) StopTask(_ context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stoppedTask = append(p.stoppedTask, id)
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

func (p *fakeProcess) SetCoAuthors(_ context.Context, people []domain.GitIdentity) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.coAuthors = people
	return nil
}

func (p *fakeProcess) credited() []domain.GitIdentity {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.coAuthors
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

func (p *fakeProcess) background(ids ...string) {
	p.events <- usecase.ProcessEvent{Kind: usecase.EventBackgroundTasks, BackgroundTasks: ids}
}

func (p *fakeProcess) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

type fakeObserver struct {
	mu     sync.Mutex
	events []usecase.SessionEvent
}

func (f *fakeObserver) Observe(_ domain.Session, e usecase.SessionEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
}

func (f *fakeObserver) of(kind usecase.SessionEventKind) []usecase.SessionEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []usecase.SessionEvent
	for _, e := range f.events {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// agentTurns are the finished turns no request waited for.
func (f *fakeObserver) agentTurns() []usecase.TurnResult {
	var out []usecase.TurnResult
	for _, e := range f.of(usecase.SessionTurnFinished) {
		if e.AgentStarted {
			out = append(out, e.Result)
		}
	}
	return out
}

type fakeRunner struct {
	mu        sync.Mutex
	started   []usecase.StartRequest
	procs     chan *fakeProcess
	leftovers []domain.SessionID
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

func (f *fakeRunner) RemoveLeftovers(context.Context) ([]domain.SessionID, error) {
	return f.leftovers, nil
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

type fakePeople map[domain.Requester]domain.GitIdentity

func (f fakePeople) GitIdentity(r domain.Requester) (domain.GitIdentity, bool) {
	id, ok := f[r]
	return id, ok
}

var (
	// alice is declared in handle_message_test.go.
	bob      = domain.Requester{Platform: "slack", ID: "U_BOB"}
	stranger = domain.Requester{Platform: "slack", ID: "U_STRANGER"}
	people   = fakePeople{
		alice: {Name: "Alice", Email: "alice@example.com"},
		bob:   {Name: "Bob", Email: "bob@example.com"},
	}
)

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
	observer   *fakeObserver
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
		observer:   &fakeObserver{},
	}
	e.workspaces.sessions = e.sessions
	e.d = usecase.NewDispatcher(fakeRegistry{dev.Name: dev, pm.Name: pm}, e.sessions, e.workspaces, e.issuer, people, e.runner, e.observer, opts)
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
	if o.err != nil || o.out.Output != "done" || o.out.Session.Turns != 1 || o.out.Session.Conversation != thread || o.out.Session.LastUsedAt.IsZero() {
		t.Fatalf("Execute() = %+v, %v", o.out, o.err)
	}
	if start := e.runner.starts()[0]; !start.Session.IsNew() || start.Credentials.GitHubToken != "token" {
		t.Errorf("process started with %+v, want a new session with credentials", start)
	}
	if e.workspaces.prepared != 1 {
		t.Errorf("workspaces prepared = %d, want 1", e.workspaces.prepared)
	}
	started, finished := e.observer.of(usecase.SessionTurnStarted), e.observer.of(usecase.SessionTurnFinished)
	if len(started) != 1 || !started[0].Status.Running || len(finished) != 1 || finished[0].AgentStarted {
		t.Errorf("observed started %+v, finished %+v", started, finished)
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

	o1, o2 := wait(t, first), wait(t, steer)
	if o1.err != nil || o1.out.Output != "both" || o1.out.Shared {
		t.Errorf("first = %+v, %v; want the turn's output", o1.out, o1.err)
	}
	if o2.err != nil || o2.out.Output != "both" || !o2.out.Shared {
		t.Errorf("injected = %+v, %v; want the same output marked as shared", o2.out, o2.err)
	}
}

func TestDispatcher_InjectedPromptTakenIntoNextTurn(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "make check"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })

	p.hold()
	steer := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "go version?", Inject: true})
	eventually(t, func() bool { return len(p.sent()) == 2 })
	p.finish("check passed")
	if o := wait(t, first); o.err != nil || o.out.Output != "check passed" || o.out.Shared {
		t.Fatalf("first = %+v, %v", o.out, o.err)
	}

	// The agent runs the held prompt as its own next turn.
	p.startTurn()
	p.accept(1)
	p.finish("go1.27")
	if o := wait(t, steer); o.err != nil || o.out.Output != "go1.27" || o.out.Shared {
		t.Errorf("injected = %+v, %v; want the next turn's output", o.out, o.err)
	}
	if got := e.observer.agentTurns(); len(got) != 0 {
		t.Errorf("observer got agent turns %+v, want none", got)
	}
}

// A resumed session first reports the background tasks its previous process left, in a turn of
// the agent's own that runs before the sent prompt is taken in. The request's turn hides it,
// errors included, since the request's own answer replaces it.
func TestDispatcher_AgentTurnBeforeSentPrompt(t *testing.T) {
	for name, early := range map[string]usecase.TurnResult{
		"answered":      {Output: "noted the stopped tasks"},
		"usage limited": {Output: "You've hit your session limit", Error: true, UsageLimited: true},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, usecase.DispatcherOptions{})
			first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "first"})
			p := e.runner.next(t)
			eventually(t, func() bool { return len(p.sent()) == 1 })
			p.finish("one")
			wait(t, first)

			p.hold()
			second := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "second"})
			eventually(t, func() bool { return len(p.sent()) == 2 })
			p.startTurn()
			p.events <- usecase.ProcessEvent{Kind: usecase.EventTurnFinished, Result: early}
			p.startTurn()
			p.accept(1)
			p.finish("two")
			if o := wait(t, second); o.err != nil || o.out.Output != "two" || o.out.Shared {
				t.Fatalf("second = %+v, %v; want the answer of the turn that took the prompt in", o.out, o.err)
			}
			if got := e.observer.agentTurns(); len(got) != 0 {
				t.Errorf("observer got agent turns %+v, want none", got)
			}
			if started := e.observer.of(usecase.SessionTurnStarted); len(started) != 2 || started[1].AgentStarted {
				t.Errorf("observed turn starts %+v, want one for each request", started)
			}
			if finished := e.observer.of(usecase.SessionTurnFinished); len(finished) != 2 {
				t.Errorf("observed %d finished turns, want one for each request", len(finished))
			}
		})
	}
}

// A prompt injected into the agent's own turn but left for the next turn does not hide that
// turn: it was shown as the agent's, and the request's turn starts after it.
func TestDispatcher_AgentTurnLeavesInjectedPrompt(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "start a build"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("building in the background")
	wait(t, first)

	p.startTurn()
	eventually(t, func() bool { return len(e.observer.of(usecase.SessionTurnStarted)) == 2 })
	p.hold()
	steer := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "go version?", Inject: true})
	eventually(t, func() bool { return len(p.sent()) == 2 })
	p.finish("build passed")
	eventually(t, func() bool { return len(e.observer.agentTurns()) == 1 })

	p.startTurn()
	p.accept(1)
	p.finish("go1.27")
	if o := wait(t, steer); o.err != nil || o.out.Output != "go1.27" {
		t.Fatalf("injected = %+v, %v", o.out, o.err)
	}
	started := e.observer.of(usecase.SessionTurnStarted)
	if len(started) != 3 || started[0].AgentStarted || !started[1].AgentStarted || started[2].AgentStarted {
		t.Errorf("observed turn starts %+v, want the request's, the agent's and the injected request's", started)
	}
}

func TestDispatcher_StopRunningTurn(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	running := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "long task"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	queued := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "next"})
	eventually(t, func() bool { return len(e.observer.of(usecase.SessionTurnUpdated)) > 0 })

	if err := e.d.Stop(context.Background(), dev.Name, existingID); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if o := wait(t, running); o.err != nil || !o.out.Interrupted {
		t.Errorf("stopped turn = %+v, %v", o.out, o.err)
	}
	if o := wait(t, queued); !errors.Is(o.err, usecase.ErrStopped) {
		t.Errorf("queued request error = %v, want ErrStopped", o.err)
	}
	if err := e.d.Stop(context.Background(), pm.Name, existingID); !errors.Is(err, domain.ErrSessionAgentMismatch) {
		t.Errorf("Stop(other agent) error = %v, want ErrSessionAgentMismatch", err)
	}
}

func TestDispatcher_StopWaitingBackgroundTasks(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "start builds"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.background("b1", "b2")
	p.finish("building")
	wait(t, first)
	time.Sleep(20 * time.Millisecond)

	if err := e.d.Stop(context.Background(), dev.Name, existingID); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	eventually(t, func() bool { p.mu.Lock(); defer p.mu.Unlock(); return len(p.stoppedTask) == 2 })
	p.mu.Lock()
	interrupted := p.interrupted
	p.mu.Unlock()
	if interrupted {
		t.Error("an interrupt was sent although no turn was running")
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
	eventually(t, func() bool { return len(e.observer.agentTurns()) == 1 })
	if got := e.observer.agentTurns()[0].Output; got != "build passed" {
		t.Errorf("observer got %q", got)
	}
	if started := e.observer.of(usecase.SessionTurnStarted); len(started) < 2 || started[0].AgentStarted || !started[1].AgentStarted {
		t.Errorf("observed turn starts %+v, want the request's and then the agent's own", started)
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
	p.background("b1")
	p.finish("building")
	wait(t, first)

	time.Sleep(100 * time.Millisecond)
	if p.isClosed() {
		t.Fatal("the process was closed while a background task was running")
	}
	p.background()
	eventually(t, p.isClosed)
}

func TestDispatcher_CreditsRequestersOfTheTurn(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "fix", Requester: alice})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.credited()) == 1 })

	steer := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "also", Inject: true, Requester: bob})
	eventually(t, func() bool { return len(p.credited()) == 2 })
	p.finish("done")
	wait(t, first)
	wait(t, steer)

	next := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "more", Requester: stranger})
	eventually(t, func() bool { return len(p.sent()) == 3 })
	p.finish("ok")
	wait(t, next)
	if got := p.credited(); len(got) != 0 {
		t.Errorf("credited %+v for a requester without a git identity, want nobody", got)
	}
}

func TestDispatcher_ShutdownInterruptsRunningTurn(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	done := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "work"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })

	e.d.Close()

	if o := wait(t, done); !errors.Is(o.err, usecase.ErrShuttingDown) {
		t.Fatalf("Execute() error = %v, want ErrShuttingDown", o.err)
	}
	if !p.interrupted || !p.isClosed() {
		t.Errorf("interrupted = %v, closed = %v; want both", p.interrupted, p.isClosed())
	}
	finished := e.observer.of(usecase.SessionTurnFinished)
	if len(finished) != 1 || !finished[0].Result.Interrupted {
		t.Errorf("finished = %+v, want one interrupted turn", finished)
	}
	if got := e.observer.of(usecase.SessionAbandoned); len(got) != 0 {
		t.Errorf("abandoned = %+v; the requester is told instead", got)
	}
}

func TestDispatcher_ShutdownAbandonsUnattendedWork(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	done := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "start a build"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.background("build")
	p.finish("started")
	wait(t, done)
	eventually(t, func() bool { return len(e.observer.of(usecase.SessionTurnFinished)) == 1 })

	e.d.Close()

	abandoned := e.observer.of(usecase.SessionAbandoned)
	if len(abandoned) != 1 || abandoned[0].Status.BackgroundTasks != 1 {
		t.Errorf("abandoned = %+v, want one with the background task", abandoned)
	}
}

func TestDispatcher_Recover(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	e.runner.leftovers = []domain.SessionID{existingID, "11111111-1111-1111-1111-111111111111"}

	n, err := e.d.Recover(context.Background())

	if n != 2 || !errors.Is(err, domain.ErrSessionNotFound) {
		t.Errorf("Recover() = %d, %v; want 2 and the unknown session", n, err)
	}
	abandoned := e.observer.of(usecase.SessionAbandoned)
	if len(abandoned) != 1 || abandoned[0].Status != (usecase.SessionStatus{}) {
		t.Errorf("abandoned = %+v, want one with an empty status", abandoned)
	}
}

func TestDispatcher_SessionRemovedBeforeLock(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	e.sessions.onLock = func(id domain.SessionID) {
		e.sessions.onLock = nil
		e.sessions.mu.Lock()
		defer e.sessions.mu.Unlock()
		delete(e.sessions.saved, id)
	}
	done := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, CreateIfMissing: true, Prompt: "hi"})

	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("done")
	o := wait(t, done)
	if o.err != nil || o.out.Session.ID != existingID || o.out.Session.Turns != 1 || e.workspaces.prepared != 1 {
		t.Fatalf("Execute() = %+v, %v, prepared %d; want a new session with the same ID", o.out, o.err, e.workspaces.prepared)
	}
}

func TestDispatcher_SessionReplacedBeforeLock(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	e.sessions.onLock = func(id domain.SessionID) {
		e.sessions.onLock = nil
		e.sessions.mu.Lock()
		defer e.sessions.mu.Unlock()
		s := e.sessions.saved[id]
		s.Turns = 0
		e.sessions.saved[id] = s
	}
	done := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "hi"})

	p := e.runner.next(t)
	if got := e.runner.starts()[0].Session.Turns; got != 0 {
		t.Errorf("started with Turns = %d, want the record read under the lock", got)
	}
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("done")
	if o := wait(t, done); o.err != nil || o.out.Session.Turns != 1 {
		t.Fatalf("Execute() = %+v, %v", o.out, o.err)
	}
}

// A turn that never finishes still counts as using the session, so that it is not cleaned up.
func TestDispatcher_LockMarksSessionUsed(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	e.sessions.saves = 0
	done := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "hi"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	e.sessions.mu.Lock()
	saves, used := e.sessions.saves, e.sessions.saved[existingID].LastUsedAt
	e.sessions.mu.Unlock()
	if saves != 1 || time.Since(used) > time.Minute {
		t.Errorf("before the turn finished: saves = %d, LastUsedAt = %v; want the session saved as used", saves, used)
	}
	p.finish("done")
	wait(t, done)
}

func TestDispatcher_TurnEndedOnError(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	run := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "fix it"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.accept(0)
	p.events <- usecase.ProcessEvent{Kind: usecase.EventTurnFinished, Result: usecase.TurnResult{
		Output: "You've hit your session limit", Error: true, UsageLimited: true,
	}}
	if o := wait(t, run); o.err != nil || !o.out.Error || !o.out.UsageLimited || o.out.Output != "You've hit your session limit" {
		t.Errorf("turn = %+v, %v; want the usage limit passed on", o.out, o.err)
	}
}

func TestDispatcher_ProcessExitsWithBackgroundTasks(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "start a build"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.background("b1")
	p.finish("building")
	wait(t, first)

	p.err = errors.New("exit status 1")
	close(p.events)
	eventually(t, func() bool {
		updates := e.observer.of(usecase.SessionUpdated)
		return len(updates) > 0 && updates[len(updates)-1].Status.BackgroundTasks == 0
	})
}
