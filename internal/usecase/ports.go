package usecase

import (
	"context"

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
}

type EventKind int

const (
	EventTurnStarted EventKind = iota + 1
	EventTurnFinished
	EventBackgroundTasks
)

// ProcessEvent reports a change in the agent process. A turn may start without a prompt,
// when the agent resumes by itself after background work.
type ProcessEvent struct {
	Kind EventKind
	// Result is set for EventTurnFinished.
	Result TurnResult
	// BackgroundTasks is the number of background tasks still running, for EventBackgroundTasks.
	BackgroundTasks int
}

// TurnListener receives the results of turns the agent started by itself, which no request waits for.
type TurnListener interface {
	AgentTurn(ctx context.Context, s domain.Session, r TurnResult)
}

type Runner interface {
	// Start launches the session's agent process, which then waits for prompts.
	Start(ctx context.Context, r StartRequest) (Process, error)
}

// Process is one running agent process of a session. Its methods are not called concurrently.
type Process interface {
	// Send delivers a prompt. During a turn, the agent takes it into that turn.
	Send(ctx context.Context, prompt string) error
	Interrupt(ctx context.Context) error
	// Events is closed when the process exits.
	Events() <-chan ProcessEvent
	// Err reports why the process exited, once Events is closed.
	Err() error
	UpdateCredentials(ctx context.Context, creds domain.Credentials) error
	// Close asks the process to exit, stops it when ctx expires first, and releases it.
	Close(ctx context.Context) error
}
