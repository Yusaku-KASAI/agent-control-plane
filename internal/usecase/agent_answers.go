package usecase

import (
	"strings"
	"sync"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// agentAnswerGrace is how long a held answer waits for the agent's next turn after background work
// runs out, since the agent reports the end of its tasks shortly before it starts that turn.
const agentAnswerGrace = 15 * time.Second

// restartNotice tells a thread that a restart stopped its agent, with the work cut short unknown.
const restartNotice = "control-plane が再起動したため、このスレッドのエージェントを止めました。処理の途中だった場合は中断されています。" +
	"続けるには、もう一度依頼してください。"

// AgentAnswers decides what of the turns no request waits for reaches people, by the table of
// docs/architecture/app/requests.md#agent-answer-rules. It passes every event on to next, and then
// tells next what to post as a SessionNotice.
type AgentAnswers struct {
	next  SessionObserver
	grace time.Duration

	mu       sync.Mutex
	sessions map[domain.SessionID]*heldAnswer
	// releases are the held answers waiting out the grace; Flush posts them at once.
	releases map[*time.Timer]func()
}

// heldAnswer is what AgentAnswers keeps for one session.
type heldAnswer struct {
	// text is the answer of an unrequested turn that ended while background work remained; gen
	// numbers it, so that a release meant for an earlier answer does not post a later one.
	text string
	gen  int
	// limited is set while the session's last turn stopped at the usage limit and that was told.
	limited bool
}

// Dispatcher.Close finds Flush only through ObserverFlusher; missing it would lose held answers.
var _ ObserverFlusher = (*AgentAnswers)(nil)

func NewAgentAnswers(next SessionObserver) *AgentAnswers {
	return &AgentAnswers{next: next, grace: agentAnswerGrace,
		sessions: make(map[domain.SessionID]*heldAnswer), releases: make(map[*time.Timer]func())}
}

// Observe passes the event on, then posts what it decides. The lock keeps the posts of a session in
// the order decided, also against a release that comes due meanwhile.
func (a *AgentAnswers) Observe(s domain.Session, e SessionEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.next.Observe(s, e)
	h := a.sessions[s.ID]
	if h == nil {
		h = &heldAnswer{}
		a.sessions[s.ID] = h
	}
	switch d := decideAnswer(h, e); d.action {
	case answerPost:
		a.post(s, d.text)
	case answerRelease:
		a.release(s, h)
	}
}

// Flush posts the held answers still waiting out the grace, so that a shutdown does not lose the
// agent's last answer. Dispatcher.Close calls it after the last event.
func (a *AgentAnswers) Flush() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for timer, release := range a.releases {
		timer.Stop()
		release()
	}
	clear(a.releases)
}

func (a *AgentAnswers) post(s domain.Session, text string) {
	if strings.TrimSpace(text) != "" {
		a.next.Observe(s, SessionEvent{Kind: SessionNotice, Notice: text})
	}
}

// release posts the held answer unless the agent starts another turn within the grace.
func (a *AgentAnswers) release(s domain.Session, h *heldAnswer) {
	if h.text == "" {
		return
	}
	gen := h.gen
	release := func() {
		if h.gen == gen && h.text != "" {
			a.post(s, h.text)
			h.text = ""
		}
	}
	var timer *time.Timer
	timer = time.AfterFunc(a.grace, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		// Flush may have posted it already. Posting only under the lock while registered keeps
		// every post before Flush returns, and so before the bots wait for their writes.
		if _, ok := a.releases[timer]; ok {
			delete(a.releases, timer)
			release()
		}
	})
	a.releases[timer] = release
}

type answerAction int

const (
	// answerPost posts the text now.
	answerPost answerAction = iota + 1
	// answerRelease posts the held answer once the grace passes without the agent starting a turn.
	answerRelease
)

type answerDecision struct {
	action answerAction
	text   string
}

// decideAnswer applies the event to what is held for the session and tells what to post, row by row
// of the table in docs/architecture/app/requests.md#agent-answer-rules.
func decideAnswer(h *heldAnswer, e SessionEvent) answerDecision {
	switch {
	case e.Kind == SessionTurnStarted && e.AgentStarted:
		// The agent's own turn decides at its end whether the held answer still matters.
		h.gen++
	case e.Kind == SessionTurnStarted:
		// A request's turn answers it whatever happened before.
		h.text = ""
	case e.Kind == SessionTurnFinished:
		return h.finished(e)
	case e.Kind == SessionUpdated && !e.Status.Running && e.Status.BackgroundTasks == 0:
		return answerDecision{action: answerRelease}
	case e.Kind == SessionAbandoned:
		// The notice replaces the held answer, which spoke of work that is now cut short.
		h.text = ""
		return answerDecision{action: answerPost, text: abandonedNotice(e.Status)}
	}
	return answerDecision{}
}

func (h *heldAnswer) finished(e SessionEvent) answerDecision {
	notice := h.errorNotice(e)
	switch {
	case e.Result.Error || e.Result.Failed:
		// A turn that ended on an error or a failure says nothing of the work before it, so the held
		// answer is posted too. One message keeps them in order, and the notice goes first so that
		// a long answer cut to the message limit cannot cut it off.
		text := joinPosts(notice, h.text)
		h.text = ""
		return answerDecision{action: answerPost, text: text}
	case !e.Unrequested:
		// The reply to the request that joined the turn answers it.
		h.text = ""
		return answerDecision{}
	}
	output := e.Result.Output
	switch {
	case strings.TrimSpace(output) == "":
		if e.Status.BackgroundTasks == 0 {
			return answerDecision{action: answerRelease}
		}
		return answerDecision{}
	case e.Result.Interrupted, e.Status.BackgroundTasks == 0:
		// No turn follows a stopped one, nor one that left no background work.
		h.text = ""
		return answerDecision{action: answerPost, text: output}
	default:
		// While background work remains, the agent will start another turn, and this answer would
		// mostly say that it is waiting; the turn after the last task ends says the rest.
		h.text = output
		h.gen++
		return answerDecision{}
	}
}

// errorNotice tells what to post for an unrequested turn that ended on an error; it is posted at
// once, even while background work remains. Once a stop at the usage limit is told, the unrequested
// turns refused while the limit lasts, one for each background task that ends, only show in the
// status message.
func (h *heldAnswer) errorNotice(e SessionEvent) string {
	told := h.limited
	h.limited = e.Result.UsageLimited
	if !e.Unrequested || !e.Result.Error || (e.Result.UsageLimited && told) {
		return ""
	}
	return TurnErrorText(e.Result.Output, e.Result.UsageLimited)
}

func abandonedNotice(st SessionStatus) string {
	if st == (SessionStatus{}) {
		return restartNotice
	}
	return ShutdownNotice
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
