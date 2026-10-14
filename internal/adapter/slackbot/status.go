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
	heldAnswerGrace = 15 * time.Second
)

type outcome int

const (
	running outcome = iota
	done
	interrupted
	failed
	usageLimited
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
	case usageLimited:
		fmt.Fprintf(&b, "⚠️ **使用上限で停止**（%s）\n", elapsed(v.startedAt, v.finishedAt))
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
	// held is the answer of the agent's own turn that ended while background work remained;
	// heldGen numbers it, so that a release meant for an earlier answer does not post a later one.
	held    string
	heldGen int
}

func (m *statusMessage) hold(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.held = text
	m.heldGen++
}

// heldGeneration is the number of the held answer, or 0 when nothing is held.
func (m *statusMessage) heldGeneration() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.held == "" {
		return 0
	}
	return m.heldGen
}

// takeHeld returns the held answer and forgets it, if it is still the one numbered gen.
func (m *statusMessage) takeHeld(gen int) string {
	if m == nil || gen == 0 {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.heldGen != gen {
		return ""
	}
	text := m.held
	m.held = ""
	return text
}

// cancelRelease keeps the held answer but stops a release already scheduled for it.
func (m *statusMessage) cancelRelease() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.heldGen++
}

func (m *statusMessage) dropHeld() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.held = ""
}

func (b *Bot) observe(s domain.Session, e usecase.SessionEvent) {
	var m *statusMessage
	if e.Kind == usecase.SessionTurnStarted {
		m = b.statusFor(s, e)
	} else {
		b.statusMu.Lock()
		m = b.status[s.ID]
		b.statusMu.Unlock()
	}
	if m != nil && e.Kind != usecase.SessionAbandoned {
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
	case e.Kind == usecase.SessionTurnFinished:
		notice, noticed := b.errorNotice(s, e)
		switch {
		case e.Result.Error || e.Result.Failed:
			// A turn that ended on an error or a failure says nothing of the work before it, so the
			// held answer is posted, in one message with the notice to keep them in order.
			held := m.takeHeld(m.heldGeneration())
			if text := joinPosts(held, notice); noticed || held != "" {
				b.post(s, text)
			}
		case !e.AgentStarted:
			// The reply to the request that joined the turn answers it.
			m.dropHeld()
		default:
			switch agentAnswer(e) {
			case postAnswer:
				m.dropHeld()
				b.post(s, e.Result.Output)
			case holdAnswer:
				if m == nil {
					b.post(s, e.Result.Output)
					break
				}
				m.hold(e.Result.Output)
			case dropAnswer:
				switch {
				case e.Result.Interrupted:
					// A stopped turn cuts short the work the held answer spoke of.
					m.dropHeld()
				case e.Status.BackgroundTasks == 0:
					b.releaseHeld(s, m)
				}
			}
		}
	case e.Kind == usecase.SessionUpdated && !e.Status.Running && e.Status.BackgroundTasks == 0:
		b.releaseHeld(s, m)
	case e.Kind == usecase.SessionAbandoned:
		// The notice replaces the held answer, which spoke of work that is now cut short.
		m.dropHeld()
		b.post(s, abandonedText(e.Status))
	}
}

// errorNotice tells what to post for a turn the agent started by itself that ended on an error;
// it is posted at once, even while background work remains. It also remembers whether the session
// stopped at the usage limit: once told, the agent's own turns refused while the limit lasts, one
// for each background task that ends, only show in the status message.
func (b *Bot) errorNotice(s domain.Session, e usecase.SessionEvent) (string, bool) {
	b.statusMu.Lock()
	told := b.limited[s.ID]
	if e.Result.UsageLimited {
		b.limited[s.ID] = true
	} else {
		delete(b.limited, s.ID)
	}
	b.statusMu.Unlock()
	if !e.AgentStarted || !e.Result.Error || (e.Result.UsageLimited && told) {
		return "", false
	}
	return usecase.TurnErrorText(e.Result.Output, e.Result.UsageLimited), true
}

// joinPosts puts texts that must appear in order into one message.
func joinPosts(texts ...string) string {
	var parts []string
	for _, t := range texts {
		if strings.TrimSpace(t) != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// releaseHeld posts the held answer unless the agent starts another turn within the grace.
func (b *Bot) releaseHeld(s domain.Session, m *statusMessage) {
	gen := m.heldGeneration()
	if gen == 0 {
		return
	}
	release := func() {
		if text := m.takeHeld(gen); text != "" {
			b.post(s, text)
		}
	}
	b.releasesMu.Lock()
	defer b.releasesMu.Unlock()
	var timer *time.Timer
	timer = time.AfterFunc(b.heldGrace, func() {
		b.releasesMu.Lock()
		defer b.releasesMu.Unlock()
		// Posting under the lock registers the write before Wait can start waiting for writes.
		if _, ok := b.releases[timer]; ok {
			delete(b.releases, timer)
			release()
		}
	})
	b.releases[timer] = release
}

// releaseAll posts the held answers still waiting out the grace, so that a shutdown does not
// lose the agent's last answer.
func (b *Bot) releaseAll() {
	b.releasesMu.Lock()
	defer b.releasesMu.Unlock()
	for timer, release := range b.releases {
		timer.Stop()
		release()
	}
	clear(b.releases)
}

// pendingReleases is the number of held answers waiting out the grace.
func (b *Bot) pendingReleases() int {
	b.releasesMu.Lock()
	defer b.releasesMu.Unlock()
	return len(b.releases)
}

// statusFor returns the status message that follows the turn starting now. A turn the agent
// starts by itself continues the work of the last request, so it takes over that request's
// message instead of posting a new one each time background work ends.
func (b *Bot) statusFor(s domain.Session, e usecase.SessionEvent) *statusMessage {
	b.statusMu.Lock()
	defer b.statusMu.Unlock()
	prev := b.status[s.ID]
	if prev != nil && e.AgentStarted {
		// The agent's own turn decides at its end whether the held answer still matters.
		prev.cancelRelease()
		prev.mu.Lock()
		prev.view.outcome, prev.view.finishedAt = running, time.Time{}
		prev.mu.Unlock()
		return prev
	}
	// A request's turn answers it whatever happened before, so the held answer is dropped.
	prev.dropHeld()
	m := &statusMessage{conv: s.Conversation, wake: make(chan struct{}, 1)}
	m.view = statusView{session: s.ID, startedAt: e.Status.TurnStartedAt}
	b.status[s.ID] = m
	return m
}

type answerAction int

const (
	dropAnswer answerAction = iota
	postAnswer
	// holdAnswer keeps the answer until the background work ends without starting another turn.
	holdAnswer
)

// agentAnswer decides what becomes of the answer of a turn the agent started by itself. While
// background work remains, the agent will start another turn, and its answer would mostly say
// that it is waiting; the turn after the last background task ends says the rest. A stopped
// turn is answered at once, since no turn follows it.
func agentAnswer(e usecase.SessionEvent) answerAction {
	switch {
	case e.Result.Failed, strings.TrimSpace(e.Result.Output) == "":
		return dropAnswer
	case e.Result.Interrupted, e.Status.BackgroundTasks == 0:
		return postAnswer
	default:
		return holdAnswer
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
	case r.UsageLimited:
		return usageLimited
	case r.Failed, r.Error:
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
