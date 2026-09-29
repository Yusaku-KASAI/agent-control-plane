package usecase

import (
	"context"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

type AgentRegistry interface {
	// FindByName returns an error wrapping domain.ErrAgentNotFound when no agent has the name.
	FindByName(ctx context.Context, name domain.AgentName) (domain.Agent, error)
}

type Runner interface {
	// Run executes one non-interactive agent session and returns its final output.
	Run(ctx context.Context, agent domain.Agent, prompt string) (string, error)
}
