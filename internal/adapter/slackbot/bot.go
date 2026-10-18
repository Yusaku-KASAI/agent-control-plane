package slackbot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

const platform = "slack"

// Bot receives app mentions for one agent over Socket Mode and answers in the thread.
type Bot struct {
	agent     domain.AgentName
	appToken  string
	botTokens TokenSource
	sm        *socketmode.Client
	handler   *usecase.HandleMessage
	log       *slog.Logger

	webMu    sync.Mutex
	webToken string
	webAPI   *slack.Client

	statusMu sync.Mutex
	status   map[domain.SessionID]*statusMessage
	// limited holds the sessions whose last turn stopped at the usage limit.
	limited    map[domain.SessionID]bool
	byStatusTS map[string]domain.SessionID
	mentions   *seenMessages
	// writes tracks messages written outside a request, such as status updates.
	writes sync.WaitGroup
	// post writes outside a request; tests replace it.
	post func(s domain.Session, text string)
	// heldGrace is how long a held answer waits for the agent's next turn after background work
	// runs out, since the agent reports the end of its tasks shortly before it starts that turn.
	heldGrace time.Duration
	// releases holds the held answers waiting out heldGrace; Wait posts them at once.
	releasesMu sync.Mutex
	releases   map[*time.Timer]func()
}

// TokenSource supplies the current bot token, which may rotate.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

func New(agent domain.AgentName, appToken string, botTokens TokenSource, turns usecase.TurnExecutor, policy usecase.AccessPolicy, log *slog.Logger) *Bot {
	b := &Bot{
		agent:     agent,
		appToken:  appToken,
		botTokens: botTokens,
		log:       log.With("agent", agent),

		status:     make(map[domain.SessionID]*statusMessage),
		limited:    make(map[domain.SessionID]bool),
		byStatusTS: make(map[string]domain.SessionID),
		mentions:   newSeenMessages(time.Hour),
		heldGrace:  heldAnswerGrace,
		releases:   make(map[*time.Timer]func()),
	}
	b.post = b.postAsync
	b.handler = usecase.NewHandleMessage(turns, policy, b)
	return b
}

// Run blocks until ctx is done, then waits for the turns it started. Those turns are not
// cancelled with ctx: they end when the dispatcher closes, which tells their requesters why.
func (b *Bot) Run(ctx context.Context) error {
	api, err := b.web(ctx)
	if err != nil {
		return err
	}
	auth, err := api.AuthTestContext(ctx)
	if err != nil {
		return fmt.Errorf("slack auth for agent %q: %w", b.agent, err)
	}
	// Socket Mode connects with the app-level token, so it keeps working when the bot token rotates.
	b.sm = socketmode.New(api)

	var turns sync.WaitGroup
	defer turns.Wait()
	errc := make(chan error, 1)
	go func() { errc <- b.sm.RunContext(ctx) }()

	for {
		select {
		case err := <-errc:
			return err
		case evt := <-b.sm.Events:
			switch inner := b.accept(evt); e := inner.(type) {
			case *slackevents.AppMentionEvent:
				msg, ok := messageFromMention(b.agent, b.team(evt), e, auth.UserID)
				if !ok || !b.mentions.first(e.Channel+"/"+e.TimeStamp, time.Now()) {
					continue
				}
				turns.Go(func() {
					if err := b.handler.Execute(context.WithoutCancel(ctx), msg); err != nil {
						b.log.Warn("message handling failed", "session", msg.Conversation.SessionID(b.agent), "err", err)
					}
				})
			case *slackevents.ReactionAddedEvent:
				b.handleReaction(ctx, e)
			}
		}
	}
}

// Wait posts the held answers still waiting out their grace, then waits for every write outside
// a request. Call it after the dispatcher is closed, so that no session event comes afterwards.
func (b *Bot) Wait() {
	b.releaseAll()
	b.writes.Wait()
}

// accept acknowledges an Events API envelope and returns its inner event.
func (b *Bot) accept(evt socketmode.Event) any {
	if evt.Type != socketmode.EventTypeEventsAPI {
		return nil
	}
	if evt.Request != nil {
		if err := b.sm.Ack(*evt.Request); err != nil {
			b.log.Warn("ack failed", "err", err)
		}
		if evt.Request.RetryAttempt > 0 {
			b.log.Info("slack redelivered an event", "attempt", evt.Request.RetryAttempt, "reason", evt.Request.RetryReason)
		}
	}
	api, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return nil
	}
	return api.InnerEvent.Data
}

func (b *Bot) team(evt socketmode.Event) string {
	api, _ := evt.Data.(slackevents.EventsAPIEvent)
	return api.TeamID
}

func (b *Bot) handleReaction(ctx context.Context, e *slackevents.ReactionAddedEvent) {
	if e.Reaction != stopReaction || e.Item.Type != "message" {
		return
	}
	id, ok := b.sessionForStatus(e.Item.Channel, e.Item.Timestamp)
	if !ok {
		return
	}
	if err := b.handler.Stop(ctx, b.agent, domain.Requester{Platform: platform, ID: e.User}, id); err != nil {
		b.log.Warn("stop failed", "session", id, "user", e.User, "err", err)
	}
}

// Observe follows the agent's sessions with a status message in their thread.
func (b *Bot) Observe(s domain.Session, e usecase.SessionEvent) {
	if s.Conversation.Platform != platform {
		return
	}
	b.observe(s, e)
}

func messageFromMention(agent domain.AgentName, team string, e *slackevents.AppMentionEvent, botUserID string) (usecase.IncomingMessage, bool) {
	if e.BotID != "" || e.User == "" {
		return usecase.IncomingMessage{}, false
	}
	thread := e.ThreadTimeStamp
	if thread == "" {
		thread = e.TimeStamp
	}
	text := strings.TrimSpace(strings.ReplaceAll(e.Text, "<@"+botUserID+">", ""))
	_, inject := strings.CutPrefix(text, injectPrefix)
	return usecase.IncomingMessage{
		Agent:        agent,
		Conversation: domain.Conversation{Platform: platform, Workspace: team, Channel: e.Channel, Thread: thread},
		Requester:    domain.Requester{Platform: platform, ID: e.User},
		MessageID:    e.TimeStamp,
		Text:         requestPrompt(e.User, e.Channel, e.TimeStamp, thread),
		Inject:       inject,
	}, true
}

// injectPrefix marks a message to be sent into the running turn.
const injectPrefix = ":zap:"

// requestPrompt points the agent at the message instead of quoting it: the agent reads the
// request, and the thread around it, from Slack (docs/adr/app/0020).
func requestPrompt(user, channel, ts, thread string) string {
	return fmt.Sprintf(`Slack のスレッドで依頼を受けました。依頼の本文はこのメッセージには含めていません。
slack スキルの手順で次のメッセージを読み、その依頼に応えてください。スレッドのほかの発言やリンク先は、必要に応じて読んでください。

- 依頼したユーザー: <@%s>
- 依頼のメッセージ: channel %s / ts %s
- スレッド: channel %s / thread_ts %s

最後の回答は control-plane がこのスレッドに投稿します。回答を自分で投稿しないでください。`, user, channel, ts, channel, thread)
}

func (b *Bot) Acknowledge(ctx context.Context, msg usecase.IncomingMessage) error {
	return b.react(ctx, msg, "eyes")
}

func (b *Bot) MarkQueued(ctx context.Context, msg usecase.IncomingMessage) error {
	return b.react(ctx, msg, "inbox_tray")
}

func (b *Bot) MarkInjected(ctx context.Context, msg usecase.IncomingMessage) error {
	return b.react(ctx, msg, "zap")
}

func (b *Bot) MarkStopped(ctx context.Context, msg usecase.IncomingMessage) error {
	return b.react(ctx, msg, stopReaction)
}

func (b *Bot) MarkAnsweredTogether(ctx context.Context, msg usecase.IncomingMessage) error {
	return b.react(ctx, msg, "white_check_mark")
}

func (b *Bot) react(ctx context.Context, msg usecase.IncomingMessage, name string) error {
	ctx = context.WithoutCancel(ctx)
	api, err := b.web(ctx)
	if err != nil {
		return err
	}
	return api.AddReactionContext(ctx, name, slack.ItemRef{Channel: msg.Conversation.Channel, Timestamp: msg.MessageID})
}

// web returns a Web API client for the current bot token.
func (b *Bot) web(ctx context.Context) (*slack.Client, error) {
	token, err := b.botTokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	b.webMu.Lock()
	defer b.webMu.Unlock()
	if token != b.webToken {
		b.webToken, b.webAPI = token, slack.New(token, slack.OptionAppLevelToken(b.appToken))
	}
	return b.webAPI, nil
}

func (b *Bot) Reply(ctx context.Context, msg usecase.IncomingMessage, text string) error {
	return b.Post(ctx, msg.Conversation, text)
}

// Post writes text to the conversation's thread.
func (b *Bot) Post(ctx context.Context, c domain.Conversation, text string) error {
	// A turn cancelled by shutdown should still be able to say so.
	ctx = context.WithoutCancel(ctx)
	api, err := b.web(ctx)
	if err != nil {
		return err
	}
	_, _, err = api.PostMessageContext(ctx, c.Channel,
		slack.MsgOptionTS(c.Thread),
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
