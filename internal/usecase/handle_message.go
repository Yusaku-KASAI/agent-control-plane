package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	// Inject sends the text into the running turn instead of queueing it.
	Inject bool
}

type AccessPolicy interface {
	Allows(agent domain.AgentName, r domain.Requester) bool
}

// ChatResponder answers in the conversation the message came from.
type ChatResponder interface {
	// Acknowledge signals that the message was received. Platforms that cannot may do nothing.
	Acknowledge(ctx context.Context, msg IncomingMessage) error
	// MarkStopped signals that the request was stopped or withdrawn. Platforms that cannot may do nothing.
	MarkStopped(ctx context.Context, msg IncomingMessage) error
	// MarkAnsweredTogether signals that the reply to an earlier request also answers this one.
	MarkAnsweredTogether(ctx context.Context, msg IncomingMessage) error
	Reply(ctx context.Context, msg IncomingMessage, text string) error
}

type TurnExecutor interface {
	Execute(ctx context.Context, in RunAgentInput) (RunAgentOutput, error)
	Stop(ctx context.Context, agent domain.AgentName, id domain.SessionID) error
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
		Inject:          msg.Inject,
		Conversation:    msg.Conversation,
	})
	switch {
	case errors.Is(err, ErrStopped):
		return errors.Join(ackErr, u.responder.MarkStopped(ctx, msg), u.responder.Reply(ctx, msg, "停止したため、この依頼は取り消しました。"))
	case err != nil:
		return errors.Join(err, ackErr, u.responder.Reply(ctx, msg, failureText(out.Session, err)))
	case out.Interrupted:
		text := "依頼を中断しました。"
		if strings.TrimSpace(out.Output) != "" {
			text = out.Output
		}
		return errors.Join(ackErr, u.responder.MarkStopped(ctx, msg), u.responder.Reply(ctx, msg, text))
	case out.Shared:
		return errors.Join(ackErr, u.responder.MarkAnsweredTogether(ctx, msg))
	}
	return errors.Join(ackErr, u.responder.Reply(ctx, msg, out.Output))
}

// Stop stops the session if the requester may use the agent.
func (u *HandleMessage) Stop(ctx context.Context, agent domain.AgentName, r domain.Requester, id domain.SessionID) error {
	if !u.policy.Allows(agent, r) {
		return domain.ErrRequesterNotAllowed
	}
	return u.turns.Stop(ctx, agent, id)
}

func failureText(s domain.Session, err error) string {
	switch {
	case errors.Is(err, domain.ErrSessionBusy):
		return "このスレッドのセッションは別の control-plane が使っています。"
	case errors.Is(err, ErrInboxFull):
		return "このスレッドには処理待ちの依頼が多すぎます。少し待ってからもう一度依頼してください。"
	case errors.Is(err, ErrEmptyPrompt):
		return "依頼の内容が空です。"
	case s.ID != "":
		return fmt.Sprintf("依頼の処理に失敗しました（セッション %s）。\n\n%s", s.ID, err)
	default:
		return fmt.Sprintf("依頼の処理に失敗しました。\n\n%s", err)
	}
}
