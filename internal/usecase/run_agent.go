package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

var ErrEmptyPrompt = errors.New("prompt is empty")

type RunAgentInput struct {
	AgentName domain.AgentName
	Prompt    string
}

type RunAgentOutput struct {
	Output string
}

type RunAgent struct {
	agents AgentRegistry
	runner Runner
}

func NewRunAgent(agents AgentRegistry, runner Runner) *RunAgent {
	return &RunAgent{agents: agents, runner: runner}
}

func (u *RunAgent) Execute(ctx context.Context, in RunAgentInput) (RunAgentOutput, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return RunAgentOutput{}, ErrEmptyPrompt
	}
	agent, err := u.agents.FindByName(ctx, in.AgentName)
	if err != nil {
		return RunAgentOutput{}, fmt.Errorf("find agent %q: %w", in.AgentName, err)
	}
	out, err := u.runner.Run(ctx, agent, in.Prompt)
	if err != nil {
		return RunAgentOutput{}, fmt.Errorf("run agent %q: %w", agent.Name, err)
	}
	return RunAgentOutput{Output: out}, nil
}
