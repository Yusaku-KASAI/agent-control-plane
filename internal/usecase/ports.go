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
	// Prepare creates the session's workspace from the agent's target.
	Prepare(ctx context.Context, agent domain.Agent, id domain.SessionID) (domain.Workspace, error)
}

type Runner interface {
	// Run executes one non-interactive turn of the session and returns its final output.
	Run(ctx context.Context, agent domain.Agent, s domain.Session, prompt string) (string, error)
}
