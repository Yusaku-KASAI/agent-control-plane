package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	// unlockedSaves counts the saves made without holding the session's lock.
	unlockedSaves int
	saveErr       error
	// onRemove runs when Remove removes a session.
	onRemove func(id domain.SessionID)
	// onLock runs before Lock takes the lock.
	onLock func(id domain.SessionID)
	// exclusive refuses a lock the test already holds, as the store does.
	exclusive bool
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
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved[s.ID] = s
	f.saves++
	if f.locks[s.ID] == 0 {
		f.unlockedSaves++
	}
	return nil
}

func (f *fakeSessions) Lock(_ context.Context, id domain.SessionID) (func(), error) {
	if f.onLock != nil {
		f.onLock(id)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.busy || f.held[id] || (f.exclusive && f.locks[id] > 0) {
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
	if f.onRemove != nil {
		f.onRemove(id)
	}
	return nil
}

type fakeWorkspaces struct {
	err      error
	prepared int
	sessions *fakeSessions
	// made are the workspaces left in place, which a clone into the same place refuses, until the
	// session is removed.
	made map[domain.SessionID]bool
}

func (f *fakeWorkspaces) Prepare(_ context.Context, _ domain.Agent, id domain.SessionID, _ domain.Credentials) (domain.Workspace, error) {
	if !f.sessions.isLocked(id) {
		return domain.Workspace{}, errors.New("prepared without the lock")
	}
	if f.made[id] {
		return domain.Workspace{}, errors.New("destination path already exists")
	}
	if f.made == nil {
		f.made = make(map[domain.SessionID]bool)
	}
	f.made[id] = true
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
	// closeErr is what Close returns, as when the process had to be stopped.
	closeErr error
	// sendErr is what Send returns, without recording the prompt.
	sendErr error
	// sendGate, when set, holds Send until it is closed; blockedSends counts the Sends held by it.
	sendGate     chan struct{}
	blockedSends int
	events       chan usecase.ProcessEvent
	err          error
}

// Send records the prompt last, so a test that has seen it may close the events channel.
func (p *fakeProcess) Send(_ context.Context, id, prompt string) error {
	p.mu.Lock()
	hold, sendErr, gate := p.holdPrompts, p.sendErr, p.sendGate
	if gate != nil {
		p.blockedSends++
	}
	p.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if sendErr != nil {
		return sendErr
	}
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
	return p.closeErr
}

func (p *fakeProcess) sent() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.prompts...)
}

func (p *fakeProcess) finish(output string) {
	p.events <- usecase.ProcessEvent{Kind: usecase.EventTurnFinished, Result: usecase.TurnResult{Output: output}}
}

// finishSpending ends the turn with the session's total the agent reports.
func (p *fakeProcess) finishSpending(output string, total float64) {
	p.events <- usecase.ProcessEvent{Kind: usecase.EventTurnFinished, Result: usecase.TurnResult{Output: output, Usage: domain.Usage{CostUSD: total}}}
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

type fakeUsage struct {
	mu       sync.Mutex
	recorded []usecase.TurnRecord
}

func (f *fakeUsage) Record(_ context.Context, u usecase.TurnRecord) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recorded = append(f.recorded, u)
}

func (f *fakeUsage) all() []usecase.TurnRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]usecase.TurnRecord(nil), f.recorded...)
}

type fakeRunner struct {
	mu        sync.Mutex
	started   []usecase.StartRequest
	procs     chan *fakeProcess
	leftovers []domain.SessionID
	// holdPrompts makes the processes it starts hold prompts from the start.
	holdPrompts bool
	// gate, when set, keeps Start from returning until it is closed.
	gate chan struct{}
}

func newFakeRunner() *fakeRunner { return &fakeRunner{procs: make(chan *fakeProcess, 8)} }

func (f *fakeRunner) Start(_ context.Context, r usecase.StartRequest) (usecase.Process, error) {
	f.mu.Lock()
	p := &fakeProcess{events: make(chan usecase.ProcessEvent, 8), holdPrompts: f.holdPrompts}
	f.started = append(f.started, r)
	gate := f.gate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
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
	usage      *fakeUsage
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
		usage:      &fakeUsage{},
	}
	e.workspaces.sessions = e.sessions
	e.sessions.onRemove = func(id domain.SessionID) { delete(e.workspaces.made, id) }
	e.d = usecase.NewDispatcher(fakeRegistry{dev.Name: dev, pm.Name: pm}, e.sessions, e.workspaces, e.issuer, people, e.runner, e.observer, e.usage, opts)
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

// uptakes records what Execute tells RunAgentInput.Taken. Read it after Execute returns.
type uptakes []usecase.Uptake

func (u *uptakes) record(up usecase.Uptake) { *u = append(*u, up) }

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
	var firstTaken, secondTaken uptakes
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "first", Taken: firstTaken.record})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })

	second := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "second", Taken: secondTaken.record})
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
	if !slices.Equal(firstTaken, uptakes{usecase.TakenIntoNewTurn}) || !slices.Equal(secondTaken, uptakes{usecase.TakenIntoNewTurn}) {
		t.Errorf("taken first %v, second %v; want each into a new turn", firstTaken, secondTaken)
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

	var steerTaken uptakes
	steer := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "also this", Inject: true, Taken: steerTaken.record})
	eventually(t, func() bool { return len(p.sent()) == 2 })
	p.finish("both")

	o1, o2 := wait(t, first), wait(t, steer)
	if o1.err != nil || o1.out.Output != "both" || o1.out.Shared {
		t.Errorf("first = %+v, %v; want the turn's output", o1.out, o1.err)
	}
	if o2.err != nil || o2.out.Output != "both" || !o2.out.Shared {
		t.Errorf("injected = %+v, %v; want the same output marked as shared", o2.out, o2.err)
	}
	if !slices.Equal(steerTaken, uptakes{usecase.TakenIntoRunningTurn}) {
		t.Errorf("taken %v, want into the running turn", steerTaken)
	}
}

// A prompt marked to be injected begins a turn of its own when there is no running turn to inject
// it into, and is not taken in when sending it into the running turn fails.
func TestDispatcher_InjectedPromptWithoutRunningTurn(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	var idleTaken uptakes
	idle := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "first", Inject: true, Taken: idleTaken.record})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })

	p.mu.Lock()
	p.sendErr = errors.New("stdin closed")
	p.mu.Unlock()
	var failedTaken uptakes
	failed := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "also", Inject: true, Taken: failedTaken.record})
	if o := wait(t, failed); o.err == nil {
		t.Errorf("injecting into a process that cannot take prompts returned %+v, want an error", o.out)
	}
	p.finish("done")
	if o := wait(t, idle); o.err != nil || o.out.Output != "done" || o.out.Shared {
		t.Fatalf("first = %+v, %v", o.out, o.err)
	}
	if !slices.Equal(idleTaken, uptakes{usecase.TakenIntoNewTurn}) || len(failedTaken) != 0 {
		t.Errorf("taken %v for the prompt sent at once and %v for the one that failed; want a new turn and nothing", idleTaken, failedTaken)
	}
}

func TestDispatcher_InjectedPromptTakenIntoNextTurn(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "make check"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })

	p.hold()
	var steerTaken uptakes
	steer := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "go version?", Inject: true, Taken: steerTaken.record})
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
	if !slices.Equal(steerTaken, uptakes{usecase.TakenIntoNewTurn}) {
		t.Errorf("taken %v, want into a new turn: the agent left it for its next turn", steerTaken)
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

// A prompt injected while the agent runs a turn of its own before taking in the sent prompt joins
// the request's turn: that early turn is hidden as part of it, so the injected prompt counts as
// taken into the turn it was injected into.
func TestDispatcher_InjectedDuringAgentTurnBeforeSentPrompt(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "first"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("one")
	wait(t, first)

	p.hold()
	var queuedTaken, steerTaken uptakes
	queued := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "second", Taken: queuedTaken.record})
	eventually(t, func() bool { return len(p.sent()) == 2 })
	p.startTurn()
	steer := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "also", Inject: true, Taken: steerTaken.record})
	eventually(t, func() bool { return len(p.sent()) == 3 })
	p.events <- usecase.ProcessEvent{Kind: usecase.EventTurnFinished, Result: usecase.TurnResult{Output: "noted the stopped tasks"}}
	p.startTurn()
	p.accept(1)
	p.accept(2)
	p.finish("both")
	if o := wait(t, queued); o.err != nil || o.out.Output != "both" || o.out.Shared {
		t.Fatalf("queued = %+v, %v", o.out, o.err)
	}
	if o := wait(t, steer); o.err != nil || !o.out.Shared {
		t.Fatalf("injected = %+v, %v; want the request's answer, shared", o.out, o.err)
	}
	if !slices.Equal(queuedTaken, uptakes{usecase.TakenIntoNewTurn}) || !slices.Equal(steerTaken, uptakes{usecase.TakenIntoRunningTurn}) {
		t.Errorf("taken %v for the queued prompt and %v for the injected one; want a new turn and the running turn", queuedTaken, steerTaken)
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
	var steerTaken uptakes
	steer := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "go version?", Inject: true, Taken: steerTaken.record})
	eventually(t, func() bool { return len(p.sent()) == 2 })
	p.finish("build passed")
	eventually(t, func() bool { return len(e.observer.agentTurns()) == 1 })

	// Another turn of the agent's own before the prompt is taken in is hidden, as for any prompt.
	p.startTurn()
	p.finish("lint passed")
	p.startTurn()
	p.accept(1)
	p.finish("go1.27")
	if o := wait(t, steer); o.err != nil || o.out.Output != "go1.27" {
		t.Fatalf("injected = %+v, %v", o.out, o.err)
	}
	if got := e.observer.agentTurns(); len(got) != 1 {
		t.Errorf("observer got agent turns %+v, want only the one the prompt was injected into", got)
	}
	if !slices.Equal(steerTaken, uptakes{usecase.TakenIntoNewTurn}) {
		t.Errorf("taken %v, want into a new turn: the agent's turn it was injected into ended without it", steerTaken)
	}
	started := e.observer.of(usecase.SessionTurnStarted)
	if len(started) != 3 || started[0].AgentStarted || !started[1].AgentStarted || started[2].AgentStarted {
		t.Errorf("observed turn starts %+v, want the request's, the agent's and the injected request's", started)
	}
}

func TestDispatcher_ActivityAndRemarks(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	out := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "long task"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.events <- usecase.ProcessEvent{Kind: usecase.EventActivity, Activity: "Bash: make check"}
	p.events <- usecase.ProcessEvent{Kind: usecase.EventActivity, Remark: "原因が分かりました"}
	eventually(t, func() bool { return len(e.observer.of(usecase.SessionTurnUpdated)) == 2 })
	updated := e.observer.of(usecase.SessionTurnUpdated)
	if updated[0].Remark != "" || updated[0].Status.Activity != "Bash: make check" {
		t.Errorf("tool call update = %+v", updated[0])
	}
	if updated[1].Remark != "原因が分かりました" || updated[1].Status.Activity != "Bash: make check" {
		t.Errorf("remark update = %+v, want the remark with the activity kept", updated[1])
	}
	p.finish("done")
	wait(t, out)
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

func (p *fakeProcess) wasInterrupted() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.interrupted
}

func (p *fakeProcess) stoppedTasks() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.stoppedTask)
}

// A request waiting in the queue is withdrawn alone; the running turn and the other waiting
// requests go on.
func TestDispatcher_StopRequestWaitingInQueue(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	running := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "long task", RequestID: "1.0"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	withdrawn := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "never mind", RequestID: "2.0"})
	kept := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "next", RequestID: "3.0"})
	eventually(t, func() bool { return len(e.observer.of(usecase.SessionTurnUpdated)) >= 2 })

	if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "2.0"); err != nil {
		t.Fatalf("StopRequest() error = %v", err)
	}
	if o := wait(t, withdrawn); !errors.Is(o.err, usecase.ErrStopped) {
		t.Errorf("withdrawn request error = %v, want ErrStopped", o.err)
	}
	if p.wasInterrupted() {
		t.Error("withdrawing a waiting request interrupted the running turn")
	}
	p.finish("done")
	if o := wait(t, running); o.err != nil || o.out.Output != "done" {
		t.Errorf("running = %+v, %v", o.out, o.err)
	}
	eventually(t, func() bool { return len(p.sent()) == 2 })
	if got := p.sent(); got[1] != "next" {
		t.Errorf("sent %q after the turn, want the request still waiting", got)
	}
	p.finish("next done")
	if o := wait(t, kept); o.err != nil || o.out.Output != "next done" {
		t.Errorf("kept = %+v, %v", o.out, o.err)
	}
}

// A request the agent works on stops the agent's work: the running turn and the background
// tasks. Requests waiting in the queue stay and go on after it.
func TestDispatcher_StopRequestTheAgentWorksOn(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	running := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "long task", RequestID: "1.0"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.background("b1")
	kept := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "next", RequestID: "2.0"})
	eventually(t, func() bool { return len(e.observer.of(usecase.SessionTurnUpdated)) >= 2 })

	if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "1.0"); err != nil {
		t.Fatalf("StopRequest() error = %v", err)
	}
	if o := wait(t, running); o.err != nil || !o.out.Interrupted {
		t.Errorf("stopped request = %+v, %v; want its turn interrupted", o.out, o.err)
	}
	if got := p.stoppedTasks(); !slices.Equal(got, []string{"b1"}) {
		t.Errorf("stopped tasks %q, want the background task", got)
	}
	eventually(t, func() bool { return len(p.sent()) == 2 })
	p.finish("next done")
	if o := wait(t, kept); o.err != nil || o.out.Output != "next done" {
		t.Errorf("kept = %+v, %v; want the waiting request to go on", o.out, o.err)
	}
}

// A request sent into the running turn but not yet taken in is given to the agent, so stopping it
// stops the agent's work and withdraws it.
func TestDispatcher_StopRequestSentButNotTakenIn(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	running := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "long task", RequestID: "1.0"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.hold()
	var steerTaken uptakes
	steer := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "also", Inject: true, RequestID: "2.0", Taken: steerTaken.record})
	eventually(t, func() bool { return len(p.sent()) == 2 })
	// A later request, so the stopped one is not the latest.
	kept := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "next", RequestID: "3.0"})
	eventually(t, func() bool { return len(e.observer.of(usecase.SessionTurnUpdated)) >= 1 })

	if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "2.0"); err != nil {
		t.Fatalf("StopRequest() error = %v", err)
	}
	if o := wait(t, steer); !errors.Is(o.err, usecase.ErrStopped) {
		t.Errorf("stopped request error = %v, want ErrStopped", o.err)
	}
	if o := wait(t, running); o.err != nil || !o.out.Interrupted {
		t.Errorf("running = %+v, %v; want its turn interrupted", o.out, o.err)
	}
	if len(steerTaken) != 0 {
		t.Errorf("taken %v, want nothing", steerTaken)
	}
	eventually(t, func() bool { return len(p.sent()) == 3 })
	p.accept(2)
	p.finish("next done")
	if o := wait(t, kept); o.err != nil || o.out.Output != "next done" {
		t.Errorf("kept = %+v, %v; want the waiting request to go on", o.out, o.err)
	}
}

// A request still in the inbox channel, as while the worker starts a process, is withdrawn too.
// The worker may take the stop before the request, so the test repeats.
func TestDispatcher_StopRequestStillInInboxChannel(t *testing.T) {
	for range 20 {
		e := newEnv(t, usecase.DispatcherOptions{})
		e.runner.gate = make(chan struct{})
		first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "first", RequestID: "1.0"})
		eventually(t, func() bool { return len(e.runner.starts()) == 1 })
		withdrawn := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "never mind", RequestID: "2.0"})
		// No event tells that the request reached the inbox channel; the worker is held in Start.
		time.Sleep(20 * time.Millisecond)
		if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "2.0"); err != nil {
			t.Fatal(err)
		}
		close(e.runner.gate)
		p := e.runner.next(t)
		if o := wait(t, withdrawn); !errors.Is(o.err, usecase.ErrStopped) {
			t.Fatalf("request stopped in the inbox channel = %+v, %v; want ErrStopped", o.out, o.err)
		}
		eventually(t, func() bool { return len(p.sent()) == 1 })
		p.finish("one")
		wait(t, first)
	}
}

// An injected request still in the inbox channel while a turn runs is sent to the agent before
// it is looked for, so stopping it stops the agent's work, as for a request the agent has.
func TestDispatcher_StopInjectedRequestStillInInboxChannel(t *testing.T) {
	// The worker takes the request from the inbox channel or the stop first, as select picks.
	for range 20 {
		e := newEnv(t, usecase.DispatcherOptions{})
		running := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "long task", RequestID: "1.0"})
		p := e.runner.next(t)
		eventually(t, func() bool { return len(p.sent()) == 1 })
		p.hold()
		gate := make(chan struct{})
		// A failure must not leave the worker held in Send, or Close waits for it.
		release := sync.OnceFunc(func() { close(gate) })
		t.Cleanup(release)
		p.mu.Lock()
		p.sendGate = gate
		p.mu.Unlock()
		held := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "also", Inject: true, RequestID: "2.0"})
		eventually(t, func() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.blockedSends == 1 })
		stopped := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "never mind", Inject: true, RequestID: "3.0"})
		// No event tells that the request reached the inbox channel; the worker is held in Send.
		time.Sleep(20 * time.Millisecond)
		if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "3.0"); err != nil {
			t.Fatalf("StopRequest() error = %v", err)
		}
		release()
		if o := wait(t, running); o.err != nil || !o.out.Interrupted {
			t.Errorf("running = %+v, %v; want its turn interrupted", o.out, o.err)
		}
		for name, ch := range map[string]<-chan outcome{"stopped": stopped, "held": held} {
			if o := wait(t, ch); !errors.Is(o.err, usecase.ErrStopped) {
				t.Errorf("%s request = %+v, %v; want ErrStopped", name, o.out, o.err)
			}
		}
	}
}

// The worker moves requests from the inbox channel to its queue as they come, so the requests
// waiting behind a turn have no limit.
func TestDispatcher_QueueHasNoLimit(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "first"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	const waiting = usecase.InboxSize + 8
	var rest []<-chan outcome
	for i := range waiting {
		rest = append(rest, e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: fmt.Sprint(i)}))
		eventually(t, func() bool {
			updates := e.observer.of(usecase.SessionTurnUpdated)
			return len(updates) > 0 && updates[len(updates)-1].Status.Queued == i+1
		})
	}
	p.finish("one")
	wait(t, first)
	for i, ch := range rest {
		eventually(t, func() bool { return len(p.sent()) == i+2 })
		p.finish(fmt.Sprint(i))
		if o := wait(t, ch); o.err != nil {
			t.Fatalf("request %d waiting behind a turn: %v", i, o.err)
		}
	}
}

// Only the requests that come while the worker cannot take them, as while it starts the agent,
// fill the inbox channel, and those past its size are refused.
func TestDispatcher_InboxOverflowsWhileTheWorkerIsHeld(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	e.runner.gate = make(chan struct{})
	release := sync.OnceFunc(func() { close(e.runner.gate) })
	// A failure must not leave the worker held, or closing the dispatcher would wait for it.
	t.Cleanup(release)
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "first"})
	eventually(t, func() bool { return len(e.runner.starts()) == 1 })
	const inbox, over = usecase.InboxSize, 8
	var rest []<-chan outcome
	for i := range inbox + over {
		rest = append(rest, e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: fmt.Sprint(i)}))
	}
	var refused []<-chan outcome
	eventually(t, func() bool {
		for i := 0; i < len(rest); i++ {
			select {
			case o := <-rest[i]:
				if !errors.Is(o.err, usecase.ErrInboxFull) {
					t.Fatalf("a request answered while the worker was held: %+v, %v", o.out, o.err)
				}
				refused = append(refused, rest[i])
				rest = slices.Delete(rest, i, i+1)
				i--
			default:
			}
		}
		return len(refused) == over
	})
	release()
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("one")
	wait(t, first)
	// The held requests went in while the worker was held, in no set order.
	for i := range rest {
		eventually(t, func() bool { return len(p.sent()) == i+2 })
		p.finish(fmt.Sprint(i))
	}
	for _, ch := range rest {
		if o := wait(t, ch); o.err != nil {
			t.Fatalf("request held in the inbox channel: %v", o.err)
		}
	}
}

// While the agent works on its own after answering, a request waiting behind it or withdrawn does
// not take the place of the latest request given to the agent: stopping that one stops the work,
// and the waiting request goes on.
func TestDispatcher_StopOwnWorkWithRequestWaiting(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "watch CI", RequestID: "1.0"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("watching")
	wait(t, first)
	p.startTurn()
	eventually(t, func() bool { return len(e.observer.of(usecase.SessionTurnStarted)) == 2 })
	withdrawn := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "never mind", RequestID: "2.0"})
	waiting := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "next", RequestID: "3.0"})
	eventually(t, func() bool { return len(e.observer.of(usecase.SessionTurnUpdated)) >= 2 })
	if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "2.0"); err != nil {
		t.Fatal(err)
	}
	wait(t, withdrawn)
	if p.wasInterrupted() {
		t.Fatal("withdrawing a waiting request interrupted the agent's own turn")
	}

	if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "1.0"); err != nil {
		t.Fatal(err)
	}
	eventually(t, p.wasInterrupted)
	eventually(t, func() bool { return len(p.sent()) == 2 })
	p.finish("next done")
	if o := wait(t, waiting); o.err != nil || o.out.Output != "next done" {
		t.Errorf("waiting = %+v, %v; want it to go on", o.out, o.err)
	}
}

// The latest request, once answered, stops the turn the agent then starts on its own.
func TestDispatcher_StopRequestWhileTheAgentWorksOnItsOwn(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "watch CI", RequestID: "1.0"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("watching")
	wait(t, first)
	p.startTurn()
	eventually(t, func() bool { return len(e.observer.of(usecase.SessionTurnStarted)) == 2 })

	if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "1.0"); err != nil {
		t.Fatalf("StopRequest() error = %v", err)
	}
	eventually(t, p.wasInterrupted)
}

// An ended request that is still the latest the worker knows stops nothing while the running turn
// is another request's: one withdrawn before, or one without an ID (from the HTTP API).
func TestDispatcher_StopEndedRequestLeavesOtherRequestsTurn(t *testing.T) {
	t.Run("withdrawn again", func(t *testing.T) {
		e := newEnv(t, usecase.DispatcherOptions{})
		running := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "long task", RequestID: "1.0"})
		p := e.runner.next(t)
		eventually(t, func() bool { return len(p.sent()) == 1 })
		withdrawn := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "never mind", RequestID: "2.0"})
		eventually(t, func() bool { return len(e.observer.of(usecase.SessionTurnUpdated)) >= 1 })
		if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "2.0"); err != nil {
			t.Fatal(err)
		}
		wait(t, withdrawn)
		if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "2.0"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
		if p.wasInterrupted() {
			t.Error("stopping the withdrawn request again interrupted the other request's turn")
		}
		p.finish("done")
		wait(t, running)
	})
	t.Run("request without an ID", func(t *testing.T) {
		e := newEnv(t, usecase.DispatcherOptions{})
		first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "first", RequestID: "1.0"})
		p := e.runner.next(t)
		eventually(t, func() bool { return len(p.sent()) == 1 })
		p.finish("one")
		wait(t, first)
		fromAPI := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "from the CLI"})
		eventually(t, func() bool { return len(p.sent()) == 2 })
		if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "1.0"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
		if p.wasInterrupted() {
			t.Error("stopping the ended Slack request interrupted the turn of a request from the HTTP API")
		}
		p.finish("two")
		wait(t, fromAPI)
	})
}

// A request that has ended stops nothing while the agent is idle, and stops the background tasks
// the agent then runs on its own, whichever ended request it is.
func TestDispatcher_StopRequestThatEnded(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "first", RequestID: "1.0"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("one")
	wait(t, first)

	if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "1.0"); err != nil {
		t.Fatalf("StopRequest() error = %v", err)
	}
	second := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "start a build", RequestID: "2.0"})
	eventually(t, func() bool { return len(p.sent()) == 2 })
	if p.wasInterrupted() {
		t.Fatal("stopping an ended request while the agent was idle interrupted the next request")
	}
	p.background("b1")
	p.finish("building")
	wait(t, second)

	if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "1.0"); err != nil {
		t.Fatalf("StopRequest() error = %v", err)
	}
	eventually(t, func() bool { return slices.Equal(p.stoppedTasks(), []string{"b1"}) })
	if p.wasInterrupted() {
		t.Error("an interrupt was sent although no turn was running")
	}
	if err := e.d.StopRequest(context.Background(), pm.Name, existingID, "2.0"); !errors.Is(err, domain.ErrSessionAgentMismatch) {
		t.Errorf("StopRequest(other agent) error = %v, want ErrSessionAgentMismatch", err)
	}
}

// While the agent works on its own, a prompt injected and not yet taken in does not keep an ended
// request from stopping that work; the injected prompt is withdrawn with it.
func TestDispatcher_StopOwnWorkWithInjectedPrompt(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "watch CI", RequestID: "1.0"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("watching")
	wait(t, first)
	p.startTurn()
	eventually(t, func() bool { return len(e.observer.of(usecase.SessionTurnStarted)) == 2 })
	p.hold()
	steer := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "also", Inject: true, RequestID: "2.0"})
	eventually(t, func() bool { return len(p.sent()) == 2 })

	if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "1.0"); err != nil {
		t.Fatal(err)
	}
	eventually(t, p.wasInterrupted)
	if o := wait(t, steer); !errors.Is(o.err, usecase.ErrStopped) {
		t.Errorf("injected request error = %v, want ErrStopped", o.err)
	}
}

// Once the agent takes an injected prompt into the turn it started on its own, that turn has a
// request, so an ended request no longer stops it.
func TestDispatcher_StopEndedRequestLeavesAgentTurnWithInjectedPrompt(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "watch CI", RequestID: "1.0"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("watching")
	wait(t, first)
	p.startTurn()
	eventually(t, func() bool { return len(e.observer.of(usecase.SessionTurnStarted)) == 2 })
	var steerTaken uptakes
	steer := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "also", Inject: true, RequestID: "2.0", Taken: steerTaken.record})
	eventually(t, func() bool { return len(p.sent()) == 2 })

	if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "1.0"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if p.wasInterrupted() {
		t.Fatal("an ended request stopped the turn the injected request was taken into")
	}
	p.finish("CI passed")
	if o := wait(t, steer); o.err != nil || o.out.Output != "CI passed" {
		t.Errorf("injected = %+v, %v", o.out, o.err)
	}
}

// A turn the agent runs before taking in a sent prompt belongs to that request, so an ended
// request does not stop it.
func TestDispatcher_StopEndedRequestLeavesAgentTurnBeforeSentPrompt(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "first", RequestID: "1.0"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("one")
	wait(t, first)
	p.hold()
	second := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "second", RequestID: "2.0"})
	eventually(t, func() bool { return len(p.sent()) == 2 })
	p.startTurn()

	if err := e.d.StopRequest(context.Background(), dev.Name, existingID, "1.0"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if p.wasInterrupted() {
		t.Fatal("an ended request stopped the turn that belongs to the sent request")
	}
	p.finish("noted")
	p.startTurn()
	p.accept(1)
	p.finish("two")
	if o := wait(t, second); o.err != nil || o.out.Output != "two" {
		t.Errorf("second = %+v, %v", o.out, o.err)
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

// A worker kept from an earlier turn marks the session used as a turn begins, and as one ends
// without finishing, so that the cleanup does not take work changed since as told of before.
func TestDispatcher_TurnsMarkSessionUsed(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	used := func() time.Time {
		e.sessions.mu.Lock()
		defer e.sessions.mu.Unlock()
		return e.sessions.saved[existingID].LastUsedAt
	}
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "hi"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("done")
	wait(t, first)
	finished := used()

	second := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "crash"})
	eventually(t, func() bool { return len(p.sent()) == 2 })
	eventually(t, func() bool { return used().After(finished) })
	began := used()
	p.err = errors.New("exit status 1")
	close(p.events)
	wait(t, second)
	if !used().After(began) {
		t.Errorf("LastUsedAt = %v after the turn failed, want later than its start %v", used(), began)
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

func TestDispatcher_FailedCreationLeavesNothing(t *testing.T) {
	tests := []struct {
		name    string
		breakIt func(env)
		fix     func(env)
	}{
		{
			name:    "credentials",
			breakIt: func(e env) { e.issuer.err = errors.New("no token") },
			fix:     func(e env) { e.issuer.err = nil },
		},
		{
			name:    "workspace",
			breakIt: func(e env) { e.workspaces.err = errors.New("fetch failed") },
			fix:     func(e env) { e.workspaces.err = nil },
		},
		{
			name:    "record",
			breakIt: func(e env) { e.sessions.saveErr = errors.New("disk full") },
			fix:     func(e env) { e.sessions.saveErr = nil },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const id domain.SessionID = "33333333-3333-5333-8333-333333333333"
			e := newEnv(t, usecase.DispatcherOptions{})
			in := usecase.RunAgentInput{AgentName: dev.Name, SessionID: id, CreateIfMissing: true, Prompt: "hi"}

			tt.breakIt(e)
			if _, err := e.d.Execute(context.Background(), in); err == nil {
				t.Fatal("Execute() error = nil, want the creation to fail")
			}
			if !slices.Equal(e.sessions.removed, []domain.SessionID{id}) {
				t.Errorf("removed = %v, want the half-made %s", e.sessions.removed, id)
			}
			if e.sessions.isLocked(id) {
				t.Error("the failed creation kept the lock")
			}

			tt.fix(e)
			retry := e.execute(in)
			p := e.runner.next(t)
			eventually(t, func() bool { return len(p.sent()) == 1 })
			p.finish("done")
			if o := wait(t, retry); o.err != nil || o.out.Session.ID != id {
				t.Fatalf("retry = %+v, %v", o.out, o.err)
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
	e.sessions.mu.Lock()
	began := e.sessions.saved[existingID].LastUsedAt
	e.sessions.mu.Unlock()

	e.d.Close()

	if o := wait(t, done); !errors.Is(o.err, usecase.ErrShuttingDown) {
		t.Fatalf("Execute() error = %v, want ErrShuttingDown", o.err)
	}
	// The interrupted turn may have changed the work since it began.
	e.sessions.mu.Lock()
	ended := e.sessions.saved[existingID].LastUsedAt
	e.sessions.mu.Unlock()
	if !ended.After(began) {
		t.Errorf("LastUsedAt = %v after the interrupted turn, want later than its start %v", ended, began)
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

// A request counts as using the session as its lock is taken, before any turn begins, so that it
// is not cleaned up.
func TestDispatcher_LockMarksSessionUsed(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	e.sessions.saves = 0
	gate := make(chan struct{})
	e.runner.mu.Lock()
	e.runner.gate = gate
	e.runner.mu.Unlock()
	done := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "hi"})
	eventually(t, func() bool { return len(e.runner.starts()) == 1 })
	e.sessions.mu.Lock()
	saves, used := e.sessions.saves, e.sessions.saved[existingID].LastUsedAt
	e.sessions.mu.Unlock()
	if saves != 1 || time.Since(used) > time.Minute {
		t.Errorf("before the turn began: saves = %d, LastUsedAt = %v; want the session saved as used", saves, used)
	}
	close(gate)
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("done")
	wait(t, done)
}

// A background task that ends with no turn, as one stopped, counts as using the session.
func TestDispatcher_BackgroundEndMarksSessionUsed(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	used := func() time.Time {
		e.sessions.mu.Lock()
		defer e.sessions.mu.Unlock()
		return e.sessions.saved[existingID].LastUsedAt
	}
	done := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "hi"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.background("bash1")
	p.finish("done")
	wait(t, done)
	finished := used()
	p.background()
	eventually(t, func() bool { return used().After(finished) })
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

func TestDispatcher_RecordsTurnUsage(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	thread := domain.Conversation{Platform: "slack", Workspace: "T1", Channel: "C1", Thread: "1.0"}
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, Prompt: "start", Conversation: thread, Requester: alice})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finishSpending("building in the background", 0.5)
	o := wait(t, first)
	id := o.out.Session.ID

	p.startTurn()
	p.finishSpending("build passed", 0.75)
	eventually(t, func() bool { return len(e.usage.all()) == 2 })

	second := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: id, Prompt: "next", Requester: bob})
	eventually(t, func() bool { return len(p.sent()) == 2 })
	p.finishSpending("ok", 2)
	wait(t, second)

	got := e.usage.all()
	want := []struct {
		cost         float64
		requesters   []domain.Requester
		agentStarted bool
	}{
		{0.5, []domain.Requester{alice}, false},
		// A turn the agent starts by itself continues the previous requesters' work.
		{0.25, []domain.Requester{alice}, true},
		{1.25, []domain.Requester{bob}, false},
	}
	if len(got) != len(want) {
		t.Fatalf("recorded %d turns, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		u := got[i]
		if u.Usage == nil || u.Usage.CostUSD != w.cost || !slices.Equal(u.Requesters, w.requesters) || u.AgentStarted != w.agentStarted ||
			u.Agent != dev.Name || u.Session != id || u.Conversation != thread || u.StartedAt.IsZero() || u.FinishedAt.Before(u.StartedAt) {
			t.Errorf("turn %d = %+v, want cost %v by %v, agent started %v", i, u, w.cost, w.requesters, w.agentStarted)
		}
	}
	if s, _ := e.sessions.Find(context.Background(), id); s.ReportedUsage == nil || s.ReportedUsage.CostUSD != 2 {
		t.Errorf("saved session = %+v, want the last reported total", s)
	}
}

func TestDispatcher_TurnUsageUnknownWithoutTotal(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	// The existing session has finished a turn but its total is not known.
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "a"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finishSpending("ok", 3)
	wait(t, first)
	second := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "b"})
	eventually(t, func() bool { return len(p.sent()) == 2 })
	p.finishSpending("ok", 4)
	wait(t, second)

	got := e.usage.all()
	if len(got) != 2 || got[0].Usage != nil || got[1].Usage == nil || got[1].Usage.CostUSD != 1 {
		t.Errorf("recorded %+v, want an unknown turn and then one costing 1", got)
	}
}

func TestDispatcher_ProcessExitForgetsTotal(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "a"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finishSpending("ok", 3)
	wait(t, first)
	eventually(t, func() bool {
		s, _ := e.sessions.Find(context.Background(), existingID)
		return s.ReportedUsage != nil
	})

	p.err = errors.New("exit status 1")
	close(p.events)
	eventually(t, func() bool {
		s, _ := e.sessions.Find(context.Background(), existingID)
		return s.ReportedUsage == nil
	})
}

func TestDispatcher_RecoverForgetsTotal(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	s := e.sessions.saved[existingID]
	s.ReportedUsage = &domain.Usage{CostUSD: 3}
	e.sessions.saved[existingID] = s
	e.runner.leftovers = []domain.SessionID{existingID}

	if _, err := e.d.Recover(context.Background()); err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if s, _ := e.sessions.Find(context.Background(), existingID); s.ReportedUsage != nil || s.Turns != 1 {
		t.Errorf("session = %+v, want its total unknown", s)
	}
	if e.sessions.unlockedSaves != 0 {
		t.Errorf("Recover saved %d times without the lock", e.sessions.unlockedSaves)
	}
	if e.sessions.isLocked(existingID) {
		t.Error("Recover kept the session locked")
	}
}

func TestDispatcher_TotalAcrossProcesses(t *testing.T) {
	for name, tc := range map[string]struct {
		closeErr error
		want     []any
	}{
		"closed": {nil, []any{1.0, 2.0}},
		// A process stopped because it did not exit may not have saved its total.
		"stopped": {errors.New("agent did not exit in time"), []any{1.0, nil}},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, usecase.DispatcherOptions{IdleTimeout: 30 * time.Millisecond})
			first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, Prompt: "first", Requester: alice})
			p1 := e.runner.next(t)
			p1.mu.Lock()
			p1.closeErr = tc.closeErr
			p1.mu.Unlock()
			eventually(t, func() bool { return len(p1.sent()) == 1 })
			p1.finishSpending("one", 1)
			id := wait(t, first).out.Session.ID
			eventually(t, p1.isClosed)

			second := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: id, Prompt: "second", Requester: alice})
			p2 := e.runner.next(t)
			eventually(t, func() bool { return len(p2.sent()) == 1 })
			p2.finishSpending("two", 3)
			wait(t, second)

			var got []any
			for _, u := range e.usage.all() {
				if u.Usage == nil {
					got = append(got, nil)
				} else {
					got = append(got, u.Usage.CostUSD)
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("recorded costs %v, want %v", got, tc.want)
			}
		})
	}
}

// A new process's own turn before it takes the sent prompt in is credited to the prompt's
// requester, and the request's turn starts when that turn ends.
func TestDispatcher_RecordsAgentTurnBeforeSentPrompt(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{IdleTimeout: 30 * time.Millisecond})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, Prompt: "first", Requester: alice})
	p1 := e.runner.next(t)
	eventually(t, func() bool { return len(p1.sent()) == 1 })
	p1.finishSpending("one", 1)
	id := wait(t, first).out.Session.ID
	eventually(t, p1.isClosed)

	e.runner.mu.Lock()
	e.runner.holdPrompts = true
	e.runner.mu.Unlock()
	second := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: id, Prompt: "second", Requester: bob})
	p2 := e.runner.next(t)
	eventually(t, func() bool { return len(p2.sent()) == 1 })
	p2.startTurn()
	p2.finishSpending("noted the stopped tasks", 1.5)
	p2.startTurn()
	p2.accept(0)
	p2.finishSpending("two", 3)
	wait(t, second)

	got := e.usage.all()
	if len(got) != 3 {
		t.Fatalf("recorded %d turns, want 3: %+v", len(got), got)
	}
	if own := got[1]; !own.AgentStarted || !slices.Equal(own.Requesters, []domain.Requester{bob}) || own.Usage == nil || own.Usage.CostUSD != 0.5 {
		t.Errorf("agent's own turn = %+v, want it credited to bob costing 0.5", own)
	}
	if req := got[2]; req.AgentStarted || req.StartedAt.Before(got[1].FinishedAt) {
		t.Errorf("request's turn = %+v, want it to start after the agent's own turn finished at %v", req, got[1].FinishedAt)
	}
	// The observer is told what each turn spent, the hidden one included.
	var spent []usecase.Spent
	for _, ev := range e.observer.of(usecase.SessionTurnUpdated) {
		if ev.Spent != nil {
			spent = append(spent, *ev.Spent)
		}
	}
	finished := e.observer.of(usecase.SessionTurnFinished)
	if len(spent) != 1 || spent[0].Turn == nil || spent[0].Turn.CostUSD != 0.5 || spent[0].Session == nil || spent[0].Session.CostUSD != 1.5 {
		t.Errorf("hidden turn spent %+v, want 0.5 of 1.5", spent)
	}
	if last := finished[len(finished)-1].Spent; last == nil || last.Turn == nil || last.Turn.CostUSD != 1.5 || last.Session == nil || last.Session.CostUSD != 3 {
		t.Errorf("request's turn spent %+v, want 1.5 of 3", last)
	}
}

// When the process is replaced in the same worker, the new process's own turn before it takes the
// held prompts in is credited to their requesters, not to the previous process's, listed in order.
func TestDispatcher_RecordsAgentTurnInReplacedProcess(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "first", Requester: stranger})
	p1 := e.runner.next(t)
	eventually(t, func() bool { return len(p1.sent()) == 1 })
	p1.finishSpending("one", 1)
	wait(t, first)
	p1.err = errors.New("exit status 1")
	close(p1.events)
	eventually(t, p1.isClosed)

	e.runner.mu.Lock()
	e.runner.holdPrompts = true
	e.runner.mu.Unlock()
	second := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "second", Requester: bob})
	p2 := e.runner.next(t)
	eventually(t, func() bool { return len(p2.sent()) == 1 })
	steer := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "third", Requester: alice, Inject: true})
	time.Sleep(20 * time.Millisecond)
	p2.startTurn()
	eventually(t, func() bool { return len(p2.sent()) == 2 })
	p2.finishSpending("noted the stopped tasks", 1.5)
	p2.startTurn()
	p2.accept(0)
	p2.accept(1)
	p2.finishSpending("two", 3)
	wait(t, second)
	wait(t, steer)

	got := e.usage.all()
	if len(got) != 3 {
		t.Fatalf("recorded %d turns, want 3: %+v", len(got), got)
	}
	if own := got[1]; !own.AgentStarted || !slices.Equal(own.Requesters, []domain.Requester{alice, bob}) {
		t.Errorf("agent's own turn = %+v, want it credited to alice and bob in order", own)
	}
}

// Zeros reported while the session's total is not known leave the total unknown.
func TestDispatcher_SpentWithoutKnownTotal(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	first := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "a"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finishSpending("crashed", 0)
	wait(t, first)
	second := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "b"})
	eventually(t, func() bool { return len(p.sent()) == 2 })
	p.finishSpending("ok", 2)
	wait(t, second)

	finished := e.observer.of(usecase.SessionTurnFinished)
	if len(finished) != 2 {
		t.Fatalf("observed %d finished turns, want 2", len(finished))
	}
	if sp := finished[0].Spent; sp == nil || sp.Turn != nil || sp.Session != nil {
		t.Errorf("first turn spent %+v, want both unknown", sp)
	}
	if sp := finished[1].Spent; sp == nil || sp.Turn != nil || sp.Session == nil || sp.Session.CostUSD != 2 {
		t.Errorf("second turn spent %+v, want an unknown turn of a total of 2", sp)
	}
}

// A worker waiting idle lets go of the session's lock at once when asked; one at work does not.
func TestDispatcher_Release(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{IdleTimeout: time.Hour})
	ctx := context.Background()
	if released, err := e.d.Release(ctx, existingID); err != nil || !released {
		t.Fatalf("Release() of a session with no worker = %v, %v; want released", released, err)
	}

	running := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "work"})
	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	if released, err := e.d.Release(ctx, existingID); err != nil || released {
		t.Fatalf("Release() during a turn = %v, %v; want kept", released, err)
	}
	p.finish("done")
	wait(t, running)

	if released, err := e.d.Release(ctx, existingID); err != nil || !released {
		t.Fatalf("Release() of an idle worker = %v, %v; want released", released, err)
	}
	unlock, err := e.sessions.Lock(ctx, existingID)
	if err != nil {
		t.Fatalf("Lock() after Release() = %v; want the lock given up", err)
	}
	unlock()
	if !p.isClosed() {
		t.Error("the idle process was not closed")
	}
}
