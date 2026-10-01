package usecase_test

import (
	"context"
	"errors"
	"testing"

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
	saved  map[domain.SessionID]domain.Session
	busy   bool
	locked bool
}

func (f *fakeSessions) Find(_ context.Context, id domain.SessionID) (domain.Session, error) {
	s, ok := f.saved[id]
	if !ok {
		return domain.Session{}, domain.ErrSessionNotFound
	}
	return s, nil
}

func (f *fakeSessions) Save(_ context.Context, s domain.Session) error {
	f.saved[s.ID] = s
	return nil
}

func (f *fakeSessions) Lock(context.Context, domain.SessionID) (func(), error) {
	if f.busy {
		return nil, domain.ErrSessionBusy
	}
	f.locked = true
	return func() { f.locked = false }, nil
}

type fakeWorkspaces struct {
	ws       domain.Workspace
	err      error
	prepared bool
	gotCreds domain.Credentials
}

func (f *fakeWorkspaces) Prepare(_ context.Context, _ domain.Agent, _ domain.SessionID, creds domain.Credentials) (domain.Workspace, error) {
	f.prepared, f.gotCreds = true, creds
	return f.ws, f.err
}

type fakeIssuer struct {
	creds domain.Credentials
	err   error
}

func (f *fakeIssuer) Issue(context.Context, domain.Agent) (domain.Credentials, error) {
	return f.creds, f.err
}

type fakeRunner struct {
	output string
	err    error

	called   bool
	gotTurn  usecase.Turn
	heldLock bool
	sessions *fakeSessions
}

func (f *fakeRunner) Run(_ context.Context, t usecase.Turn) (string, error) {
	f.called, f.gotTurn, f.heldLock = true, t, f.sessions.locked
	return f.output, f.err
}

var (
	dev = domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: "/src/repo", Image: "repo-dev"}}
	pm  = domain.Agent{Name: "repo-pm", Role: "pm", Target: domain.Target{Path: "/src/repo", Image: "repo-dev"}}
	ws  = domain.Workspace{Path: "/sessions/x/workspace/repo"}
)

const existingID domain.SessionID = "0f8fad5b-d9cb-469f-a165-70867728950e"

type env struct {
	sessions   *fakeSessions
	workspaces *fakeWorkspaces
	issuer     *fakeIssuer
	runner     *fakeRunner
	uc         *usecase.RunAgent
}

func newEnv(runner *fakeRunner) env {
	sessions := &fakeSessions{saved: map[domain.SessionID]domain.Session{
		existingID: {ID: existingID, AgentName: dev.Name, Workspace: ws, Turns: 1},
	}}
	workspaces := &fakeWorkspaces{ws: ws}
	issuer := &fakeIssuer{creds: domain.Credentials{GitHubToken: "ghs_test"}}
	runner.sessions = sessions
	return env{
		sessions:   sessions,
		workspaces: workspaces,
		issuer:     issuer,
		runner:     runner,
		uc:         usecase.NewRunAgent(fakeRegistry{dev.Name: dev, pm.Name: pm}, sessions, workspaces, issuer, runner),
	}
}

func TestRunAgent_NewSession(t *testing.T) {
	e := newEnv(&fakeRunner{output: "done"})

	got, err := e.uc.Execute(context.Background(), usecase.RunAgentInput{AgentName: dev.Name, Prompt: "fix the bug"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Output != "done" || got.Session.Workspace != ws || got.Session.Turns != 1 {
		t.Errorf("Execute() = %+v", got)
	}
	wantTurn := usecase.Turn{Agent: dev, Session: got.Session, Prompt: "fix the bug", Credentials: e.issuer.creds}
	wantTurn.Session.Turns = 0
	if e.runner.gotTurn != wantTurn || !e.runner.heldLock {
		t.Errorf("runner got %+v (lock held: %v), want %+v under lock", e.runner.gotTurn, e.runner.heldLock, wantTurn)
	}
	if e.workspaces.gotCreds != e.issuer.creds {
		t.Errorf("workspace got credentials %+v, want %+v", e.workspaces.gotCreds, e.issuer.creds)
	}
	if e.sessions.saved[got.Session.ID] != got.Session {
		t.Errorf("saved session = %+v, want %+v", e.sessions.saved[got.Session.ID], got.Session)
	}
	if e.sessions.locked {
		t.Error("lock was not released")
	}
}

func TestRunAgent_ContinueSession(t *testing.T) {
	e := newEnv(&fakeRunner{output: "again"})

	got, err := e.uc.Execute(context.Background(), usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "and then?"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if e.workspaces.prepared {
		t.Error("a continued session prepared a new workspace")
	}
	if e.runner.gotTurn.Session.Turns != 1 || got.Session.Turns != 2 || e.sessions.saved[existingID].Turns != 2 {
		t.Errorf("turns: runner got %d, output %d, saved %d; want 1, 2, 2",
			e.runner.gotTurn.Session.Turns, got.Session.Turns, e.sessions.saved[existingID].Turns)
	}
}

func TestRunAgent_CreateIfMissing(t *testing.T) {
	const id domain.SessionID = "22222222-2222-5222-8222-222222222222"

	e := newEnv(&fakeRunner{output: "first"})
	in := usecase.RunAgentInput{AgentName: dev.Name, SessionID: id, CreateIfMissing: true, Prompt: "hi"}
	got, err := e.uc.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Session.ID != id || !e.workspaces.prepared || !e.runner.gotTurn.Session.IsNew() {
		t.Errorf("Execute() = %+v (prepared %v), want a new session with the given ID", got, e.workspaces.prepared)
	}

	e.workspaces.prepared = false
	if _, err := e.uc.Execute(context.Background(), in); err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if e.workspaces.prepared || e.runner.gotTurn.Session.Turns != 1 {
		t.Errorf("second turn prepared a workspace (%v) or did not resume (turns %d)", e.workspaces.prepared, e.runner.gotTurn.Session.Turns)
	}
}

func TestRunAgent_Errors(t *testing.T) {
	errRunner := errors.New("container failed")
	errClone := errors.New("clone failed")
	errIssue := errors.New("token request failed")

	tests := []struct {
		name      string
		in        usecase.RunAgentInput
		setup     func(env)
		wantErr   error
		wantRun   bool
		wantTurns int
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
			name:    "workspace failure",
			in:      usecase.RunAgentInput{AgentName: dev.Name, Prompt: "hi"},
			setup:   func(e env) { e.workspaces.err = errClone },
			wantErr: errClone,
		},
		{
			name:      "busy session",
			in:        usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "hi"},
			setup:     func(e env) { e.sessions.busy = true },
			wantErr:   domain.ErrSessionBusy,
			wantTurns: 1,
		},
		{
			name:    "credential failure",
			in:      usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "hi"},
			setup:   func(e env) { e.issuer.err = errIssue },
			wantErr: errIssue,
		},
		{
			name:      "runner failure does not count the turn",
			in:        usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, Prompt: "hi"},
			setup:     func(e env) { e.runner.err = errRunner },
			wantErr:   errRunner,
			wantRun:   true,
			wantTurns: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(&fakeRunner{})
			if tt.setup != nil {
				tt.setup(e)
			}
			got, err := e.uc.Execute(context.Background(), tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Execute() error = %v, want %v", err, tt.wantErr)
			}
			if e.runner.called != tt.wantRun {
				t.Errorf("runner called = %v, want %v", e.runner.called, tt.wantRun)
			}
			if got.Session.Turns != tt.wantTurns || e.sessions.saved[existingID].Turns != 1 {
				t.Errorf("turns = %d (saved %d), want %d (saved 1)", got.Session.Turns, e.sessions.saved[existingID].Turns, tt.wantTurns)
			}
		})
	}
}
