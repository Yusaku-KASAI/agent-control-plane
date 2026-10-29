package usecase

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// noticeRecorder records the notices AgentAnswers tells, and the other events it passes on.
type noticeRecorder struct {
	mu      sync.Mutex
	notices []string
	passed  int
}

func (r *noticeRecorder) Observe(_ domain.Session, e SessionEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.Kind == SessionNotice {
		r.notices = append(r.notices, e.Notice)
		return
	}
	r.passed++
}

func (r *noticeRecorder) posted() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.notices)
}

func (a *AgentAnswers) pendingReleases() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.releases)
}

func TestAgentAnswers(t *testing.T) {
	started := func(agent bool) SessionEvent {
		return SessionEvent{Kind: SessionTurnStarted, AgentStarted: agent, Status: SessionStatus{Running: true}}
	}
	finished := func(output string, background int) SessionEvent {
		return SessionEvent{Kind: SessionTurnFinished, AgentStarted: true,
			Result: TurnResult{Output: output}, Status: SessionStatus{BackgroundTasks: background}}
	}
	updated := func(background int) SessionEvent {
		return SessionEvent{Kind: SessionUpdated, Status: SessionStatus{BackgroundTasks: background}}
	}
	for _, tc := range []struct {
		name   string
		events []SessionEvent
		want   []string
	}{
		{"an answer leaving no background work",
			[]SessionEvent{started(false), started(true), finished("結果", 0)},
			[]string{"結果"}},
		{"an empty answer leaving no background work",
			[]SessionEvent{started(false), started(true), finished(" \n", 0)},
			nil},
		{"a stopped turn's answer while tasks remain",
			[]SessionEvent{started(false), started(true), {Kind: SessionTurnFinished, AgentStarted: true,
				Result: TurnResult{Output: "止めました", Interrupted: true}, Status: SessionStatus{BackgroundTasks: 1}}},
			[]string{"止めました"}},
		{"the next turn starts after the tasks run out",
			[]SessionEvent{started(false), started(true), finished("待ちます", 1), updated(0), started(true), finished("終わりました", 0)},
			[]string{"終わりました"}},
		{"an answer posted after a held one drops it",
			[]SessionEvent{started(false), started(true), finished("A", 1), started(true), finished("B", 0), started(true), finished("", 0)},
			[]string{"B"}},
		{"a turn starting within the grace stops the release",
			[]SessionEvent{started(false), started(true), finished("待ちます", 1), updated(0), started(true)},
			nil},
		{"the next turn starts before the tasks run out",
			[]SessionEvent{started(false), started(true), finished("待ちます", 1), started(true),
				{Kind: SessionTurnUpdated, Status: SessionStatus{Running: true}}, finished("終わりました", 0)},
			[]string{"終わりました"}},
		{"the tasks run out without a turn",
			[]SessionEvent{started(false), started(true), finished("最後の結果", 1), updated(1), updated(0)},
			[]string{"最後の結果"}},
		{"a request comes while an answer is held",
			[]SessionEvent{started(false), started(true), finished("待ちます", 1), started(false), updated(0)},
			nil},
		{"a release meant for an earlier answer",
			[]SessionEvent{started(false), started(true), finished("A", 1), updated(0), started(true), finished("B", 1)},
			nil},
		{"a later answer released in its own time",
			[]SessionEvent{started(false), started(true), finished("A", 1), updated(0), started(true), finished("B", 1), updated(0)},
			[]string{"B"}},
		{"an error while background work remains",
			[]SessionEvent{started(false), started(true), {Kind: SessionTurnFinished, AgentStarted: true,
				Result: TurnResult{Output: "API Error", Error: true}, Status: SessionStatus{BackgroundTasks: 1}}, updated(0)},
			[]string{TurnErrorText("API Error", false)}},
		{"the next turn stops at the usage limit",
			[]SessionEvent{started(false), started(true), finished("PR を作りました", 1), updated(0), started(true),
				{Kind: SessionTurnFinished, AgentStarted: true, Result: TurnResult{Output: "limit", Error: true, UsageLimited: true}}},
			[]string{TurnErrorText("limit", true) + "\n\n---\n\nPR を作りました"}},
		{"a turn with a request joined stops at the usage limit",
			[]SessionEvent{started(false), started(true), finished("PR を作りました", 1), updated(0), started(true),
				{Kind: SessionTurnFinished, Result: TurnResult{Output: "limit", Error: true, UsageLimited: true}}},
			[]string{"PR を作りました"}},
		{"the next turn is stopped",
			[]SessionEvent{started(false), started(true), finished("PR を作りました", 1), started(true),
				{Kind: SessionTurnFinished, AgentStarted: true, Result: TurnResult{Interrupted: true}}, updated(0)},
			[]string{"PR を作りました"}},
		{"the next turn ends without an answer while tasks remain",
			[]SessionEvent{started(false), started(true), finished("PR を作りました", 1), started(true), finished("", 1), updated(0)},
			[]string{"PR を作りました"}},
		{"the answer stays held while tasks remain after an answerless turn",
			[]SessionEvent{started(false), started(true), finished("PR を作りました", 1), started(true), finished("", 1)},
			nil},
		{"the next turn fails with the process",
			[]SessionEvent{started(false), started(true), finished("PR を作りました", 1), updated(0), started(true),
				{Kind: SessionTurnFinished, AgentStarted: true, Result: TurnResult{Failed: true}}},
			[]string{"PR を作りました"}},
		{"the next turn ends without an answer",
			[]SessionEvent{started(false), started(true), finished("PR を作りました", 1), updated(0), started(true), finished("", 0)},
			[]string{"PR を作りました"}},
		{"a request joins the next turn",
			[]SessionEvent{started(false), started(true), finished("待ちます", 1), updated(0), started(true),
				{Kind: SessionTurnFinished, Result: TurnResult{Output: "答え"}}, updated(0)},
			nil},
		{"the control plane stops while an answer is held",
			[]SessionEvent{started(false), started(true), finished("待ちます", 1), {Kind: SessionAbandoned, Status: SessionStatus{BackgroundTasks: 1}}, updated(0)},
			[]string{ShutdownNotice}},
		{"a restart abandons the work",
			[]SessionEvent{{Kind: SessionAbandoned}},
			[]string{RestartNotice}},
	} {
		next := &noticeRecorder{}
		a := NewAgentAnswers(next)
		a.grace = 20 * time.Millisecond
		s := domain.Session{ID: "sess"}
		for _, e := range tc.events {
			a.Observe(s, e)
		}
		deadline := time.Now().Add(5 * time.Second)
		for a.pendingReleases() > 0 && time.Now().Before(deadline) {
			time.Sleep(a.grace)
		}
		if got := next.posted(); !slices.Equal(got, tc.want) {
			t.Errorf("%s: posted %q, want %q", tc.name, got, tc.want)
		}
		if next.passed != len(tc.events) {
			t.Errorf("%s: passed on %d of %d events", tc.name, next.passed, len(tc.events))
		}
	}
}

func TestAgentAnswersErrorNotice(t *testing.T) {
	h := &heldAnswer{}
	limit := TurnResult{Output: "You've hit your session limit · resets 2pm", Error: true, UsageLimited: true}
	for _, st := range []struct {
		name  string
		event SessionEvent
		want  string
	}{
		{"a request stops at the limit", SessionEvent{Result: limit}, ""},
		{"the agent's own turn at the same limit", SessionEvent{AgentStarted: true, Result: limit}, ""},
		{"a turn passes", SessionEvent{AgentStarted: true, Result: TurnResult{Output: "ok"}}, ""},
		{"the agent's own turn at the limit again", SessionEvent{AgentStarted: true, Result: limit}, "> You've hit your session limit · resets 2pm"},
		{"the agent's own turn on another error", SessionEvent{AgentStarted: true, Result: TurnResult{Output: "API Error", Error: true}}, "エラーでターンが終わりました"},
		{"the agent's own turn failing with the process", SessionEvent{AgentStarted: true, Result: TurnResult{Failed: true}}, ""},
		{"the agent's own turn answering with blanks", SessionEvent{AgentStarted: true, Result: TurnResult{Output: " \n"}}, ""},
	} {
		text := h.errorNotice(st.event)
		if (text != "") != (st.want != "") || !strings.Contains(text, st.want) {
			t.Errorf("%s: notice = %q; want one containing %q", st.name, text, st.want)
		}
	}
}

func TestAgentAnswersFlush(t *testing.T) {
	held := []SessionEvent{
		{Kind: SessionTurnStarted},
		{Kind: SessionTurnStarted, AgentStarted: true},
		{Kind: SessionTurnFinished, AgentStarted: true, Result: TurnResult{Output: "最後の結果"}, Status: SessionStatus{BackgroundTasks: 1}},
	}
	for _, tc := range []struct {
		name   string
		events []SessionEvent
		want   []string
	}{
		{"the tasks have run out", append(slices.Clone(held), SessionEvent{Kind: SessionUpdated}),
			[]string{"最後の結果"}},
		{"the control plane cuts the tasks short", append(slices.Clone(held),
			SessionEvent{Kind: SessionAbandoned, Status: SessionStatus{BackgroundTasks: 1}},
			SessionEvent{Kind: SessionUpdated}),
			[]string{ShutdownNotice}},
	} {
		next := &noticeRecorder{}
		a := NewAgentAnswers(next)
		a.grace = time.Hour
		for _, e := range tc.events {
			a.Observe(domain.Session{ID: "sess"}, e)
		}
		a.Flush()
		if got := next.posted(); !slices.Equal(got, tc.want) {
			t.Errorf("%s: posted %q on shutdown, want %q", tc.name, got, tc.want)
		}
	}
}
