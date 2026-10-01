package slackbot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

const platform = "slack"

// Bot receives app mentions for one agent over Socket Mode and answers in the thread.
type Bot struct {
	agent   domain.AgentName
	api     *slack.Client
	sm      *socketmode.Client
	handler *usecase.HandleMessage
	log     *slog.Logger
}

func New(agent domain.AgentName, appToken, botToken string, turns usecase.TurnExecutor, policy usecase.AccessPolicy, log *slog.Logger) *Bot {
	api := slack.New(botToken, slack.OptionAppLevelToken(appToken))
	b := &Bot{
		agent: agent,
		api:   api,
		sm:    socketmode.New(api),
		log:   log.With("agent", agent),
	}
	b.handler = usecase.NewHandleMessage(turns, policy, b)
	return b
}

// Run blocks until ctx is done, then waits for the turns it started.
func (b *Bot) Run(ctx context.Context) error {
	auth, err := b.api.AuthTestContext(ctx)
	if err != nil {
		return fmt.Errorf("slack auth for agent %q: %w", b.agent, err)
	}

	var turns sync.WaitGroup
	defer turns.Wait()
	errc := make(chan error, 1)
	go func() { errc <- b.sm.RunContext(ctx) }()

	for {
		select {
		case err := <-errc:
			return err
		case evt := <-b.sm.Events:
			msg, ok := b.accept(evt, auth.UserID)
			if !ok {
				continue
			}
			turns.Go(func() {
				if err := b.handler.Execute(ctx, msg); err != nil {
					b.log.Warn("message handling failed", "session", msg.Conversation.SessionID(b.agent), "err", err)
				}
			})
		}
	}
}

func (b *Bot) accept(evt socketmode.Event, botUserID string) (usecase.IncomingMessage, bool) {
	if evt.Type != socketmode.EventTypeEventsAPI {
		return usecase.IncomingMessage{}, false
	}
	if evt.Request != nil {
		if err := b.sm.Ack(*evt.Request); err != nil {
			b.log.Warn("ack failed", "err", err)
		}
	}
	api, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return usecase.IncomingMessage{}, false
	}
	mention, ok := api.InnerEvent.Data.(*slackevents.AppMentionEvent)
	if !ok {
		return usecase.IncomingMessage{}, false
	}
	return messageFromMention(b.agent, api.TeamID, mention, botUserID)
}

func messageFromMention(agent domain.AgentName, team string, e *slackevents.AppMentionEvent, botUserID string) (usecase.IncomingMessage, bool) {
	if e.BotID != "" || e.User == "" {
		return usecase.IncomingMessage{}, false
	}
	thread := e.ThreadTimeStamp
	if thread == "" {
		thread = e.TimeStamp
	}
	return usecase.IncomingMessage{
		Agent:        agent,
		Conversation: domain.Conversation{Platform: platform, Workspace: team, Channel: e.Channel, Thread: thread},
		Requester:    domain.Requester{Platform: platform, ID: e.User},
		MessageID:    e.TimeStamp,
		Text:         strings.TrimSpace(strings.ReplaceAll(e.Text, "<@"+botUserID+">", "")),
	}, true
}

func (b *Bot) Acknowledge(ctx context.Context, msg usecase.IncomingMessage) error {
	return b.api.AddReactionContext(ctx, "eyes", slack.ItemRef{Channel: msg.Conversation.Channel, Timestamp: msg.MessageID})
}

func (b *Bot) Reply(ctx context.Context, msg usecase.IncomingMessage, text string) error {
	// A turn cancelled by shutdown should still be able to say so.
	_, _, err := b.api.PostMessageContext(context.WithoutCancel(ctx), msg.Conversation.Channel,
		slack.MsgOptionTS(msg.Conversation.Thread),
		slack.MsgOptionText(fallbackText(text), false),
		slack.MsgOptionBlocks(slack.NewMarkdownBlock("", markdownText(text))),
	)
	return err
}

// Slack caps a markdown block at 12,000 characters and the notification text at 40,000.
const (
	maxMarkdown = 11_000
	maxFallback = 3_000
)

func markdownText(text string) string {
	return truncate(text, maxMarkdown, "\n\n…（長いため省略しました。全文は作業領域や PR を確認してください）")
}

func fallbackText(text string) string {
	return truncate(text, maxFallback, "…")
}

func truncate(s string, limit int, suffix string) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit]) + suffix
}
