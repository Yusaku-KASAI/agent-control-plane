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
	// SessionID continues an existing session; empty starts a new one.
	SessionID domain.SessionID
	Prompt    string
}

type RunAgentOutput struct {
	Output string
	// Session is set whenever it exists, even if the turn then failed.
	Session domain.Session
}

type RunAgent struct {
	agents      AgentRegistry
	sessions    SessionRepository
	workspaces  WorkspaceProvider
	credentials CredentialIssuer
	runner      Runner
}

func NewRunAgent(
	agents AgentRegistry,
	sessions SessionRepository,
	workspaces WorkspaceProvider,
	credentials CredentialIssuer,
	runner Runner,
) *RunAgent {
	return &RunAgent{agents: agents, sessions: sessions, workspaces: workspaces, credentials: credentials, runner: runner}
}

func (u *RunAgent) Execute(ctx context.Context, in RunAgentInput) (RunAgentOutput, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return RunAgentOutput{}, ErrEmptyPrompt
	}
	agent, err := u.agents.FindByName(ctx, in.AgentName)
	if err != nil {
		return RunAgentOutput{}, fmt.Errorf("find agent %q: %w", in.AgentName, err)
	}

	session, err := u.openSession(ctx, agent, in.SessionID)
	if err != nil {
		return RunAgentOutput{}, err
	}
	unlock, err := u.sessions.Lock(ctx, session.ID)
	if err != nil {
		return RunAgentOutput{Session: session}, fmt.Errorf("lock session %s: %w", session.ID, err)
	}
	defer unlock()

	creds, err := u.credentials.Issue(ctx, agent)
	if err != nil {
		return RunAgentOutput{Session: session}, fmt.Errorf("issue credentials for agent %q: %w", agent.Name, err)
	}
	out, err := u.runner.Run(ctx, Turn{Agent: agent, Session: session, Prompt: in.Prompt, Credentials: creds})
	if err != nil {
		return RunAgentOutput{Session: session}, fmt.Errorf("run agent %q: %w", agent.Name, err)
	}
	session.Turns++
	if err := u.sessions.Save(ctx, session); err != nil {
		return RunAgentOutput{Output: out, Session: session}, fmt.Errorf("save session %s: %w", session.ID, err)
	}
	return RunAgentOutput{Output: out, Session: session}, nil
}

func (u *RunAgent) openSession(ctx context.Context, agent domain.Agent, id domain.SessionID) (domain.Session, error) {
	if id != "" {
		s, err := u.sessions.Find(ctx, id)
		if err != nil {
			return domain.Session{}, fmt.Errorf("find session %s: %w", id, err)
		}
		if err := s.CheckAgent(agent.Name); err != nil {
			return domain.Session{}, err
		}
		return s, nil
	}

	id = domain.NewRandomSessionID()
	ws, err := u.workspaces.Prepare(ctx, agent, id)
	if err != nil {
		return domain.Session{}, fmt.Errorf("prepare workspace for agent %q: %w", agent.Name, err)
	}
	s := domain.Session{ID: id, AgentName: agent.Name, Workspace: ws}
	if err := u.sessions.Save(ctx, s); err != nil {
		return domain.Session{}, fmt.Errorf("save session %s: %w", id, err)
	}
	return s, nil
}
