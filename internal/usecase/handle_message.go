package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// IncomingMessage is a request addressed to an agent, however the platform delivered it.
type IncomingMessage struct {
	Agent        domain.AgentName
	Conversation domain.Conversation
	Requester    domain.Requester
	// MessageID identifies the message itself within the conversation, for acknowledging it.
	MessageID string
	Text      string
}

type AccessPolicy interface {
	Allows(agent domain.AgentName, r domain.Requester) bool
}

// ChatResponder answers in the conversation the message came from.
type ChatResponder interface {
	// Acknowledge signals that the message was received. Platforms that cannot may do nothing.
	Acknowledge(ctx context.Context, msg IncomingMessage) error
	Reply(ctx context.Context, msg IncomingMessage, text string) error
}

type TurnExecutor interface {
	Execute(ctx context.Context, in RunAgentInput) (RunAgentOutput, error)
}

type HandleMessage struct {
	turns     TurnExecutor
	policy    AccessPolicy
	responder ChatResponder
}

func NewHandleMessage(turns TurnExecutor, policy AccessPolicy, responder ChatResponder) *HandleMessage {
	return &HandleMessage{turns: turns, policy: policy, responder: responder}
}

// Execute runs the turn and replies with its output or a description of the failure. The
// returned error is for logging; the requester has already been told.
func (u *HandleMessage) Execute(ctx context.Context, msg IncomingMessage) error {
	if !u.policy.Allows(msg.Agent, msg.Requester) {
		return errors.Join(domain.ErrRequesterNotAllowed,
			u.responder.Reply(ctx, msg, "この依頼者からの依頼は受け付けていません。"))
	}
	ackErr := u.responder.Acknowledge(ctx, msg)

	out, err := u.turns.Execute(ctx, RunAgentInput{
		AgentName:       msg.Agent,
		SessionID:       msg.Conversation.SessionID(msg.Agent),
		CreateIfMissing: true,
		Prompt:          msg.Text,
	})
	if err != nil {
		return errors.Join(err, ackErr, u.responder.Reply(ctx, msg, failureText(out.Session, err)))
	}
	return errors.Join(ackErr, u.responder.Reply(ctx, msg, out.Output))
}

func failureText(s domain.Session, err error) string {
	switch {
	case errors.Is(err, domain.ErrSessionBusy):
		return "このスレッドでは前の依頼を処理中です。終わってからもう一度依頼してください。"
	case errors.Is(err, ErrEmptyPrompt):
		return "依頼の内容が空です。"
	case s.ID != "":
		return fmt.Sprintf("依頼の処理に失敗しました（セッション %s）。\n\n%s", s.ID, err)
	default:
		return fmt.Sprintf("依頼の処理に失敗しました。\n\n%s", err)
	}
}
