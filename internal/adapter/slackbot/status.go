package slackbot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/slack-go/slack"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

const (
	stopReaction = "no_entry_sign"
	// refreshInterval keeps the elapsed time moving and batches bursts of tool calls into one edit.
	refreshInterval = 5 * time.Second
)

type outcome int

const (
	running outcome = iota
	done
	interrupted
	failed
)

// statusView is what a status message shows.
type statusView struct {
	session    domain.SessionID
	startedAt  time.Time
	finishedAt time.Time
	outcome    outcome
	activity   string
	queued     int
	background int
}

func (v statusView) render(now time.Time) string {
	var b strings.Builder
	switch v.outcome {
	case running:
		fmt.Fprintf(&b, "⏳ **実行中**（%s）\n", elapsed(v.startedAt, now))
		if v.activity != "" {
			fmt.Fprintf(&b, "今の作業：%s\n", v.activity)
		}
	case done:
		fmt.Fprintf(&b, "✅ **完了**（%s）\n", elapsed(v.startedAt, v.finishedAt))
	case interrupted:
		fmt.Fprintf(&b, "⏹ **中断**（%s）\n", elapsed(v.startedAt, v.finishedAt))
	case failed:
		fmt.Fprintf(&b, "⚠️ **失敗**（%s）\n", elapsed(v.startedAt, v.finishedAt))
	}
	if v.queued > 0 {
		fmt.Fprintf(&b, "待っている依頼：%d 件\n", v.queued)
	}
	if v.background > 0 {
		fmt.Fprintf(&b, "バックグラウンドの処理：%d 件（終わるとエージェントが再開します）\n", v.background)
	}
	fmt.Fprintf(&b, "\nセッション `%s`", v.session)
	if v.stoppable() {
		fmt.Fprintf(&b, "・止めるには、このメッセージに :%s: を付けてください", stopReaction)
	}
	return b.String()
}

func (v statusView) stoppable() bool {
	return v.outcome == running || v.queued > 0 || v.background > 0
}

func elapsed(from, to time.Time) string {
	d := to.Sub(from).Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%d秒", int(d.Seconds()))
	}
	return fmt.Sprintf("%d分%02d秒", int(d.Minutes()), int(d.Seconds())%60)
}

// statusMessage is the message in the thread that follows one turn.
type statusMessage struct {
	conv domain.Conversation

	mu       sync.Mutex
	view     statusView
	ts       string
	updating bool
	wake     chan struct{}
}

func (b *Bot) observe(s domain.Session, e usecase.SessionEvent) {
	switch e.Kind {
	case usecase.SessionTurnStarted:
		m := &statusMessage{conv: s.Conversation, wake: make(chan struct{}, 1)}
		m.view = statusView{session: s.ID, startedAt: e.Status.TurnStartedAt}
		m.apply(e.Status)
		b.statusMu.Lock()
		b.status[s.ID] = m
		b.statusMu.Unlock()
		b.refresh(m)
	case usecase.SessionTurnUpdated, usecase.SessionUpdated, usecase.SessionTurnFinished:
		b.statusMu.Lock()
		m := b.status[s.ID]
		b.statusMu.Unlock()
		if m == nil {
			return
		}
		m.mu.Lock()
		if e.Kind == usecase.SessionTurnFinished {
			m.view.finishedAt = time.Now()
			m.view.outcome = outcomeOf(e.Result)
		}
		m.mu.Unlock()
		m.apply(e.Status)
		b.refresh(m)
	}
	switch {
	case e.Kind == usecase.SessionTurnFinished && e.AgentStarted && !e.Result.Failed && strings.TrimSpace(e.Result.Output) != "":
		b.postAsync(s, e.Result.Output)
	case e.Kind == usecase.SessionAbandoned:
		b.postAsync(s, abandonedText(e.Status))
	}
}

func abandonedText(st usecase.SessionStatus) string {
	if st == (usecase.SessionStatus{}) {
		return "control-plane が再起動したため、このスレッドのエージェントを止めました。処理の途中だった場合は中断されています。" +
			"続けるには、もう一度依頼してください。"
	}
	return usecase.ShutdownNotice
}

func (b *Bot) postAsync(s domain.Session, text string) {
	b.writes.Go(func() {
		if err := b.Post(context.Background(), s.Conversation, text); err != nil {
			b.log.Warn("posting to the thread failed", "session", s.ID, "err", err)
		}
	})
}

func outcomeOf(r usecase.TurnResult) outcome {
	switch {
	case r.Failed:
		return failed
	case r.Interrupted:
		return interrupted
	default:
		return done
	}
}

func (m *statusMessage) apply(st usecase.SessionStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.view.activity = st.Activity
	m.view.queued = st.Queued
	m.view.background = st.BackgroundTasks
}

// refresh makes sure a goroutine is updating the message; it posts it first if needed.
func (b *Bot) refresh(m *statusMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updating {
		select {
		case m.wake <- struct{}{}:
		default:
		}
		return
	}
	m.updating = true
	b.writes.Go(func() { b.updateStatus(m) })
}

func (b *Bot) updateStatus(m *statusMessage) {
	ctx := context.Background()
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	var shown string
	for {
		m.mu.Lock()
		view, ts := m.view, m.ts
		m.mu.Unlock()

		if text := view.render(time.Now()); text != shown {
			if err := b.writeStatus(ctx, m, ts, text); err != nil {
				b.log.Warn("status message update failed", "session", view.session, "err", err)
			}
			shown = text
		}
		if view.outcome != running {
			m.mu.Lock()
			if m.view == view {
				m.updating = false
				m.mu.Unlock()
				return
			}
			m.mu.Unlock()
			continue
		}
		select {
		case <-m.wake:
			time.Sleep(time.Second)
		case <-ticker.C:
		}
	}
}

func (b *Bot) writeStatus(ctx context.Context, m *statusMessage, ts, text string) error {
	opts := []slack.MsgOption{
		slack.MsgOptionText(fallbackText(text), false),
		slack.MsgOptionBlocks(slack.NewMarkdownBlock("", text)),
	}
	api, err := b.web(ctx)
	if err != nil {
		return err
	}
	if ts != "" {
		_, _, _, err := api.UpdateMessageContext(ctx, m.conv.Channel, ts, opts...)
		return err
	}
	_, ts, err = api.PostMessageContext(ctx, m.conv.Channel, append(opts, slack.MsgOptionTS(m.conv.Thread))...)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.ts = ts
	m.mu.Unlock()
	b.statusMu.Lock()
	b.byStatusTS[m.conv.Channel+"/"+ts] = m.view.session
	b.statusMu.Unlock()
	return nil
}

// sessionForStatus finds the session whose status message is at channel and ts.
func (b *Bot) sessionForStatus(channel, ts string) (domain.SessionID, bool) {
	b.statusMu.Lock()
	defer b.statusMu.Unlock()
	id, ok := b.byStatusTS[channel+"/"+ts]
	return id, ok
}
