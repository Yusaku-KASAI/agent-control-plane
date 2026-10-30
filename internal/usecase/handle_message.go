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

// ChatResponder answers in the conversation the message came from. The Mark methods signal, on
// the message, how far the request has come; platforms that cannot may do nothing.
type ChatResponder interface {
	// Acknowledge signals that the request was received and waits for the agent to take it in.
	Acknowledge(ctx context.Context, msg IncomingMessage) error
	// MarkStarted signals that the agent took the request in and is working on it.
	MarkStarted(ctx context.Context, msg IncomingMessage) error
	// MarkInjected signals that the agent took the request into the turn it was injected into.
	MarkInjected(ctx context.Context, msg IncomingMessage) error
	// MarkAnswered signals that the request is done: it was answered, by its own reply or by the
	// reply to an earlier request taken into the same turn.
	MarkAnswered(ctx context.Context, msg IncomingMessage) error
	// MarkFailed signals that the request ended on an error.
	MarkFailed(ctx context.Context, msg IncomingMessage) error
	// MarkStopped signals that the request was stopped or withdrawn.
	MarkStopped(ctx context.Context, msg IncomingMessage) error
	Reply(ctx context.Context, msg IncomingMessage, text string) error
}

type TurnExecutor interface {
	Execute(ctx context.Context, in RunAgentInput) (RunAgentOutput, error)
	StopRequest(ctx context.Context, agent domain.AgentName, id domain.SessionID, requestID string) error
}

// SessionCleanup cleans one session now, as a person asked in its thread (SessionCleaner.CleanNow),
// or despite work found nowhere else, as they asked on the notice of it (SessionCleaner.ForceClean).
type SessionCleanup interface {
	CleanNow(ctx context.Context, id domain.SessionID, by domain.Requester) error
	ForceClean(ctx context.Context, agent domain.AgentName, by domain.Requester, c domain.Conversation, message string) error
}

type HandleMessage struct {
	turns     TurnExecutor
	cleanup   SessionCleanup
	policy    AccessPolicy
	responder ChatResponder
}

func NewHandleMessage(turns TurnExecutor, cleanup SessionCleanup, policy AccessPolicy, responder ChatResponder) *HandleMessage {
	return &HandleMessage{turns: turns, cleanup: cleanup, policy: policy, responder: responder}
}

// Execute runs the turn and replies with its output or a description of the failure. The
// returned error is for logging; the requester has already been told.
func (u *HandleMessage) Execute(ctx context.Context, msg IncomingMessage) error {
	if !u.policy.Allows(msg.Agent, msg.Requester) {
		return errors.Join(domain.ErrRequesterNotAllowed,
			u.responder.Reply(ctx, msg, "この依頼者からの依頼は受け付けていません。"))
	}
	markErr := u.responder.Acknowledge(ctx, msg)
	taken := func(up Uptake) {
		switch up {
		case TakenIntoNewTurn:
			markErr = errors.Join(markErr, u.responder.MarkStarted(ctx, msg))
		case TakenIntoRunningTurn:
			markErr = errors.Join(markErr, u.responder.MarkInjected(ctx, msg))
		}
	}

	out, err := u.turns.Execute(ctx, RunAgentInput{
		AgentName:       msg.Agent,
		SessionID:       msg.Conversation.SessionID(msg.Agent),
		CreateIfMissing: true,
		Prompt:          msg.Text,
		Inject:          msg.Inject,
		Conversation:    msg.Conversation,
		Requester:       msg.Requester,
		RequestID:       msg.MessageID,
		Taken:           taken,
	})
	// Each outcome replies first and then marks the message, so the mark follows its reply.
	switch {
	case errors.Is(err, ErrStopped):
		return errors.Join(markErr, u.responder.Reply(ctx, msg, "停止したため、この依頼は取り消しました。"), u.responder.MarkStopped(ctx, msg))
	case errors.Is(err, ErrShuttingDown):
		return errors.Join(markErr, u.responder.Reply(ctx, msg, ShutdownNotice), u.responder.MarkStopped(ctx, msg))
	case err != nil:
		return errors.Join(err, markErr, u.responder.Reply(ctx, msg, failureText(out.Session, err)), u.responder.MarkFailed(ctx, msg))
	case out.Interrupted:
		if strings.TrimSpace(out.Output) == "" {
			return errors.Join(markErr, u.responder.Reply(ctx, msg, "依頼を中断しました。"), u.responder.MarkStopped(ctx, msg))
		}
		return errors.Join(markErr, u.answer(ctx, msg, out, u.responder.MarkStopped))
	case out.Error && out.Shared:
		return errors.Join(markErr, u.responder.MarkFailed(ctx, msg))
	case out.Error:
		return errors.Join(markErr, u.responder.Reply(ctx, msg, TurnErrorText(out.Output, out.UsageLimited)), u.responder.MarkFailed(ctx, msg))
	case out.Shared:
		return errors.Join(markErr, u.responder.MarkAnswered(ctx, msg))
	}
	return errors.Join(markErr, u.answer(ctx, msg, out, u.responder.MarkAnswered))
}

// answer posts the turn's answer and then marks the message. An answer that cannot be posted, after
// the responder gave up retrying, is told by a short notice, which may still get through where the
// answer did not (an answer Slack refuses), and the message is marked failed instead, so that it does
// not look answered.
func (u *HandleMessage) answer(ctx context.Context, msg IncomingMessage, out RunAgentOutput, mark func(context.Context, IncomingMessage) error) error {
	err := u.responder.Reply(ctx, msg, out.Output)
	if err == nil {
		return mark(ctx, msg)
	}
	return errors.Join(err, u.responder.Reply(ctx, msg, lostAnswerText(out.Session)), u.responder.MarkFailed(ctx, msg))
}

// lostAnswerText tells that the answer could not be posted. The agent's answer stays in the session,
// so asking again in the thread has it written again.
func lostAnswerText(s domain.Session) string {
	text := "⚠️ 回答をこのスレッドに投稿できませんでした。もう一度依頼すると、エージェントが回答を書き直します。"
	if s.ID != "" {
		text += fmt.Sprintf("（セッション %s）", s.ID)
	}
	return text
}

// TurnErrorText tells that the agent ended a turn on an error, quoting the agent's message, which
// for a usage limit says when the limit resets in the agent container's time zone.
func TurnErrorText(message string, usageLimited bool) string {
	text := "⚠️ エラーでターンが終わりました。"
	if usageLimited {
		text = "⚠️ 使用上限に当たり、エージェントが止まりました。上限が戻ったあとに、もう一度依頼すると続きから進めます。"
	}
	if message = strings.TrimSpace(message); message != "" {
		text += "\n\n> " + strings.ReplaceAll(message, "\n", "\n> ")
	}
	return text
}

// ShutdownNotice tells the requester that a restart cut the work short and how to go on.
const ShutdownNotice = "control-plane の再起動のため、処理を中断しました。続けるには、もう一度依頼してください。"

// StopRequest stops one request, the message msgID in the conversation, if the requester may use
// the agent. How far the request has come decides what stops (Dispatcher.StopRequest).
func (u *HandleMessage) StopRequest(ctx context.Context, agent domain.AgentName, r domain.Requester, c domain.Conversation, msgID string) error {
	if !u.policy.Allows(agent, r) {
		return domain.ErrRequesterNotAllowed
	}
	return u.turns.StopRequest(ctx, agent, c.SessionID(agent), msgID)
}

// CleanSession cleans the conversation's session now, if the requester may use the agent
// (SessionCleaner.CleanNow).
func (u *HandleMessage) CleanSession(ctx context.Context, agent domain.AgentName, r domain.Requester, c domain.Conversation) error {
	if !u.policy.Allows(agent, r) {
		return domain.ErrRequesterNotAllowed
	}
	return u.cleanup.CleanNow(ctx, c.SessionID(agent), r)
}

// ForceCleanSession cleans, despite work found nowhere else, the session whose notice of that work
// is the message in the conversation's channel, if the requester may use the agent
// (SessionCleaner.ForceClean).
func (u *HandleMessage) ForceCleanSession(ctx context.Context, agent domain.AgentName, r domain.Requester, c domain.Conversation, message string) error {
	if !u.policy.Allows(agent, r) {
		return domain.ErrRequesterNotAllowed
	}
	return u.cleanup.ForceClean(ctx, agent, r, c, message)
}

func failureText(s domain.Session, err error) string {
	switch {
	case errors.Is(err, domain.ErrSessionBusy):
		return "このスレッドのセッションは、ほかの処理（セッションの作成、片付け、別の control-plane）が使っています。少し待ってからもう一度依頼してください。"
	case errors.Is(err, ErrEmptyPrompt):
		return "依頼の内容が空です。"
	case s.ID != "":
		return fmt.Sprintf("依頼の処理に失敗しました（セッション %s）。\n\n%s", s.ID, err)
	default:
		return fmt.Sprintf("依頼の処理に失敗しました。\n\n%s", err)
	}
}
