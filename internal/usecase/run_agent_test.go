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

type fakeRunner struct {
	output string
	err    error

	gotAgent  domain.Agent
	gotPrompt string
}

func (f *fakeRunner) Run(_ context.Context, agent domain.Agent, prompt string) (string, error) {
	f.gotAgent, f.gotPrompt = agent, prompt
	return f.output, f.err
}

func TestRunAgent_Execute(t *testing.T) {
	dev := domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: "/src/repo", Image: "repo-dev"}}
	registry := fakeRegistry{dev.Name: dev}
	errRunner := errors.New("container failed")

	tests := []struct {
		name       string
		in         usecase.RunAgentInput
		runner     *fakeRunner
		wantOutput string
		wantErr    error
	}{
		{
			name:       "runs the named agent",
			in:         usecase.RunAgentInput{AgentName: dev.Name, Prompt: "fix the bug"},
			runner:     &fakeRunner{output: "done"},
			wantOutput: "done",
		},
		{
			name:    "empty prompt",
			in:      usecase.RunAgentInput{AgentName: dev.Name, Prompt: "  "},
			runner:  &fakeRunner{},
			wantErr: usecase.ErrEmptyPrompt,
		},
		{
			name:    "unknown agent",
			in:      usecase.RunAgentInput{AgentName: "nobody", Prompt: "hi"},
			runner:  &fakeRunner{},
			wantErr: domain.ErrAgentNotFound,
		},
		{
			name:    "runner failure",
			in:      usecase.RunAgentInput{AgentName: dev.Name, Prompt: "hi"},
			runner:  &fakeRunner{err: errRunner},
			wantErr: errRunner,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := usecase.NewRunAgent(registry, tt.runner).Execute(context.Background(), tt.in)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Execute() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got.Output != tt.wantOutput {
				t.Errorf("Output = %q, want %q", got.Output, tt.wantOutput)
			}
			if tt.runner.gotAgent != dev || tt.runner.gotPrompt != tt.in.Prompt {
				t.Errorf("runner got (%+v, %q), want (%+v, %q)", tt.runner.gotAgent, tt.runner.gotPrompt, dev, tt.in.Prompt)
			}
		})
	}
}
