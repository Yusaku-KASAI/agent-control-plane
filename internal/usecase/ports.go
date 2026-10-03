package usecase

import (
	"context"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

type AgentRegistry interface {
	// FindByName returns an error wrapping domain.ErrAgentNotFound when no agent has the name.
	FindByName(ctx context.Context, name domain.AgentName) (domain.Agent, error)
}

type SessionRepository interface {
	// Find returns an error wrapping domain.ErrSessionNotFound when the session does not exist.
	Find(ctx context.Context, id domain.SessionID) (domain.Session, error)
	Save(ctx context.Context, s domain.Session) error
	// Lock returns an error wrapping domain.ErrSessionBusy while another turn holds the session.
	Lock(ctx context.Context, id domain.SessionID) (unlock func(), err error)
}

type WorkspaceProvider interface {
	// Prepare creates the session's workspace from the agent's target, using creds to reach its remote.
	Prepare(ctx context.Context, agent domain.Agent, id domain.SessionID, creds domain.Credentials) (domain.Workspace, error)
}

type CredentialIssuer interface {
	// Issue returns credentials valid for at least one turn.
	Issue(ctx context.Context, agent domain.Agent) (domain.Credentials, error)
}

type StartRequest struct {
	Agent       domain.Agent
	Session     domain.Session
	Credentials domain.Credentials
}

type TurnResult struct {
	Output string
	// Interrupted is set when the turn ended because of Process.Interrupt.
	Interrupted bool
	// Failed is set when the process ended before the turn did.
	Failed bool
}

type EventKind int

const (
	EventTurnStarted EventKind = iota + 1
	EventTurnFinished
	EventBackgroundTasks
	EventActivity
	// EventPromptAccepted reports that the agent took the prompt into the running turn.
	EventPromptAccepted
)

// ProcessEvent reports a change in the agent process. A turn may start without a prompt,
// when the agent resumes by itself after background work.
type ProcessEvent struct {
	Kind EventKind
	// Result is set for EventTurnFinished.
	Result TurnResult
	// BackgroundTasks lists the IDs of background tasks still running, for EventBackgroundTasks.
	BackgroundTasks []string
	// Activity is what the agent is doing now, for EventActivity.
	Activity string
	// PromptID is set for EventPromptAccepted.
	PromptID string
}

type SessionEventKind int

const (
	SessionTurnStarted SessionEventKind = iota + 1
	SessionTurnUpdated
	SessionTurnFinished
	// SessionUpdated reports a change between turns, such as background tasks ending.
	SessionUpdated
)

type SessionEvent struct {
	Kind   SessionEventKind
	Status SessionStatus
	// Result and AgentStarted are set for SessionTurnFinished. No request waits for a turn the
	// agent started by itself, so its result reaches people only through the observer.
	Result       TurnResult
	AgentStarted bool
}

type SessionStatus struct {
	Running         bool
	TurnStartedAt   time.Time
	Activity        string
	Queued          int
	BackgroundTasks int
}

// SessionObserver is told how a session's turns progress. Calls come from the session's worker
// in order and must return quickly.
type SessionObserver interface {
	Observe(s domain.Session, e SessionEvent)
}

type Runner interface {
	// Start launches the session's agent process, which then waits for prompts.
	Start(ctx context.Context, r StartRequest) (Process, error)
}

// Process is one running agent process of a session. Its methods are not called concurrently.
type Process interface {
	// Send delivers a prompt. The agent takes it into the running turn or into a later one,
	// and reports which by EventPromptAccepted with the same id.
	Send(ctx context.Context, id, prompt string) error
	// Interrupt ends the running turn and drops the prompts not yet taken in; background tasks keep running.
	Interrupt(ctx context.Context) error
	StopTask(ctx context.Context, taskID string) error
	// Events is closed when the process exits.
	Events() <-chan ProcessEvent
	// Err reports why the process exited, once Events is closed.
	Err() error
	UpdateCredentials(ctx context.Context, creds domain.Credentials) error
	// Close asks the process to exit, stops it when ctx expires first, and releases it.
	Close(ctx context.Context) error
}
