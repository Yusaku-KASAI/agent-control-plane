package slackbot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slack-go/slack/slackevents"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

func TestMessageFromMention(t *testing.T) {
	tests := []struct {
		name   string
		event  slackevents.AppMentionEvent
		want   usecase.IncomingMessage
		wantOK bool
	}{
		{
			name:  "top-level mention starts a thread on itself",
			event: slackevents.AppMentionEvent{User: "U1", Channel: "C1", TimeStamp: "10.0", Text: "<@UBOT> fix the bug"},
			want: usecase.IncomingMessage{
				Agent:        "repo-dev",
				Conversation: domain.Conversation{Platform: "slack", Workspace: "T1", Channel: "C1", Thread: "10.0"},
				Requester:    domain.Requester{Platform: "slack", ID: "U1"},
				MessageID:    "10.0",
				Text:         requestPrompt("U1", "C1", "10.0", "10.0"),
			},
			wantOK: true,
		},
		{
			name:  "mention in a thread continues it",
			event: slackevents.AppMentionEvent{User: "U1", Channel: "C1", TimeStamp: "12.0", ThreadTimeStamp: "10.0", Text: "and <@UBOT> again"},
			want: usecase.IncomingMessage{
				Agent:        "repo-dev",
				Conversation: domain.Conversation{Platform: "slack", Workspace: "T1", Channel: "C1", Thread: "10.0"},
				Requester:    domain.Requester{Platform: "slack", ID: "U1"},
				MessageID:    "12.0",
				Text:         requestPrompt("U1", "C1", "12.0", "10.0"),
			},
			wantOK: true,
		},
		{
			name:  "zap sends into the running turn",
			event: slackevents.AppMentionEvent{User: "U1", Channel: "C1", TimeStamp: "13.0", ThreadTimeStamp: "10.0", Text: "<@UBOT> :zap: do this first"},
			want: usecase.IncomingMessage{
				Agent:        "repo-dev",
				Conversation: domain.Conversation{Platform: "slack", Workspace: "T1", Channel: "C1", Thread: "10.0"},
				Requester:    domain.Requester{Platform: "slack", ID: "U1"},
				MessageID:    "13.0",
				Text:         requestPrompt("U1", "C1", "13.0", "10.0"),
				Inject:       true,
			},
			wantOK: true,
		},
		{name: "bot messages are ignored", event: slackevents.AppMentionEvent{BotID: "B1", Channel: "C1", TimeStamp: "1.0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := messageFromMention("repo-dev", "T1", &tt.event, "UBOT")
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("messageFromMention() = %+v, %v; want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestRequestPrompt(t *testing.T) {
	got := requestPrompt("U1", "C1", "12.0", "10.0")
	for _, want := range []string{"<@U1>", "channel C1 / ts 12.0", "channel C1 / thread_ts 10.0", "slack スキル"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt lacks %q:\n%s", want, got)
		}
	}
}

func TestMarkdownText(t *testing.T) {
	if got := markdownText("short"); got != "short" {
		t.Errorf("markdownText(short) = %q", got)
	}
	long := strings.Repeat("あ", maxMarkdown+10)
	got := markdownText(long)
	if !strings.HasPrefix(got, strings.Repeat("あ", maxMarkdown)) || !strings.Contains(got, "省略") {
		t.Errorf("markdownText(long) was not truncated with a notice")
	}
}

func TestStatusViewRender(t *testing.T) {
	start := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	running := statusView{session: "sess", startedAt: start, activity: "Bash: Run lint and tests", queued: 1, background: 2}
	got := running.render(start.Add(83 * time.Second))
	for _, want := range []string{"実行中", "1分23秒", "Bash: Run lint and tests", "待っている依頼：1 件", "バックグラウンドの処理：2 件", "`sess`", ":no_entry_sign:"} {
		if !strings.Contains(got, want) {
			t.Errorf("running render lacks %q:\n%s", want, got)
		}
	}

	running.remarks = remarks("原因が分かりました")
	if got := running.render(start); !strings.Contains(got, "\nこれまでの発言（古い順）：\n- 原因が分かりました\n\n今の作業：") {
		t.Errorf("running render does not list the remark apart from the activity:\n%s", got)
	}
	running.remarks = remarks("一つ目", "二つ目")
	if got := running.render(start); !strings.Contains(got, "：\n- 一つ目\n- 二つ目\n\n今の作業：") {
		t.Errorf("running render does not list the remarks oldest first:\n%s", got)
	}

	finished := statusView{session: "sess", startedAt: start, finishedAt: start.Add(9 * time.Second), outcome: done, remarks: remarks("A", "B")}
	got = finished.render(start.Add(time.Hour))
	if strings.Contains(got, "発言") {
		t.Errorf("finished render shows remarks:\n%s", got)
	}
	if !strings.Contains(got, "完了") || !strings.Contains(got, "9秒") || strings.Contains(got, ":no_entry_sign:") {
		t.Errorf("finished render:\n%s", got)
	}

	limited := statusView{session: "sess", startedAt: start, finishedAt: start, outcome: usageLimited}
	if got := limited.render(start); !strings.Contains(got, "使用上限で停止") {
		t.Errorf("usage-limited render:\n%s", got)
	}
	if outcomeOf(usecase.TurnResult{Error: true}) != failed || outcomeOf(usecase.TurnResult{Error: true, UsageLimited: true}) != usageLimited {
		t.Error("a turn that ended on an error is not shown as failed")
	}
}

func TestErrorNotice(t *testing.T) {
	b := New("dev", "", nil, nil, nil, slog.New(slog.DiscardHandler))
	s := domain.Session{ID: "sess"}
	limit := usecase.TurnResult{Output: "You've hit your session limit · resets 2pm", Error: true, UsageLimited: true}
	steps := []struct {
		name  string
		event usecase.SessionEvent
		want  string
	}{
		{"a request stops at the limit", usecase.SessionEvent{Result: limit}, ""},
		{"the agent's own turn at the same limit", usecase.SessionEvent{AgentStarted: true, Result: limit}, ""},
		{"a turn passes", usecase.SessionEvent{AgentStarted: true, Result: usecase.TurnResult{Output: "ok"}}, ""},
		{"the agent's own turn at the limit again", usecase.SessionEvent{AgentStarted: true, Result: limit}, "> You've hit your session limit · resets 2pm"},
		{"the agent's own turn on another error", usecase.SessionEvent{AgentStarted: true, Result: usecase.TurnResult{Output: "API Error", Error: true}}, "エラーでターンが終わりました"},
		{"the agent's own turn failing with the process", usecase.SessionEvent{AgentStarted: true, Result: usecase.TurnResult{Failed: true}}, ""},
		{"the agent's own turn answering with blanks", usecase.SessionEvent{AgentStarted: true, Result: usecase.TurnResult{Output: " \n"}}, ""},
	}
	for _, st := range steps {
		st.event.Kind = usecase.SessionTurnFinished
		text := b.errorNotice(s, st.event)
		if (text != "") != (st.want != "") || !strings.Contains(text, st.want) {
			t.Errorf("%s: notice = %q; want one containing %q", st.name, text, st.want)
		}
	}
}

func TestStatusForAgentStartedTurn(t *testing.T) {
	b := New("dev", "", nil, nil, nil, slog.New(slog.DiscardHandler))
	s := domain.Session{ID: "sess", Conversation: domain.Conversation{Platform: "slack", Channel: "C1", Thread: "1.0"}}
	start := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

	if m := b.statusFor(s, usecase.SessionEvent{Kind: usecase.SessionTurnStarted, AgentStarted: true}); m == nil {
		t.Fatal("an agent-started turn without an earlier status message got none")
	}
	requested := b.statusFor(s, usecase.SessionEvent{Kind: usecase.SessionTurnStarted, Status: usecase.SessionStatus{TurnStartedAt: start}})
	requested.view.outcome, requested.view.finishedAt = done, start.Add(time.Minute)

	resumed := b.statusFor(s, usecase.SessionEvent{Kind: usecase.SessionTurnStarted, AgentStarted: true, Status: usecase.SessionStatus{TurnStartedAt: start.Add(time.Hour)}})
	if resumed != requested {
		t.Fatal("an agent-started turn posted a new status message instead of taking over the request's")
	}
	if resumed.view.outcome != running || !resumed.view.finishedAt.IsZero() || !resumed.view.startedAt.Equal(start) {
		t.Errorf("taken-over status = %+v, want running since the request started", resumed.view)
	}

	if next := b.statusFor(s, usecase.SessionEvent{Kind: usecase.SessionTurnStarted}); next == requested {
		t.Error("a requested turn reused the earlier status message")
	}
}

func TestObserveKeepsLatestRemarks(t *testing.T) {
	b := New("dev", "", nil, nil, nil, slog.New(slog.DiscardHandler))
	s := domain.Session{ID: "sess", Conversation: domain.Conversation{Platform: "slack", Channel: "C1", Thread: "1.0"}}
	m := b.statusFor(s, usecase.SessionEvent{Kind: usecase.SessionTurnStarted})
	m.updating = true // keeps observe from starting to write to Slack
	var sent []string
	for i := range maxRemarks + 1 {
		sent = append(sent, fmt.Sprintf("%d つ目", i+1))
	}
	for _, r := range append(sent, " \n ") {
		b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnUpdated, Remark: r, Status: usecase.SessionStatus{Running: true}})
	}
	if want := remarks(sent[1:]...); m.view.remarks != want {
		t.Errorf("remarks = %q, want %q", m.view.remarks, want)
	}
	answer := "終わりました。\n\nPR は #1 です。" + strings.Repeat("あ", maxRemarkLength)
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnUpdated, Remark: answer})
	if last := m.view.remarks[maxRemarks-1]; last != remarkLine(answer) || !strings.HasPrefix(last, "終わりました。 PR は #1 です。あ") || !strings.HasSuffix(last, "あ…") {
		t.Errorf("long remark = %q, want it on one line and cut", last)
	}
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnFinished, Result: usecase.TurnResult{Output: answer}})
	if want := remarks(sent[2:]...); m.view.remarks != want {
		t.Errorf("remarks after the answer = %q, want %q without the answer", m.view.remarks, want)
	}
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnUpdated, Remark: "その前の発言"})
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnUpdated, Remark: "終わりました。"})
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnUpdated, Remark: "PR は #1 です。" + strings.Repeat("い", maxRemarkLength)})
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnFinished, Result: usecase.TurnResult{Output: "終わりました。\n\nPR は #1 です。" + strings.Repeat("い", maxRemarkLength+10)}})
	if want := remarks(append(sent[4:], "その前の発言")...); m.view.remarks != want {
		t.Errorf("remarks after an answer that came in blocks = %q, want %q without any of its blocks", m.view.remarks, want)
	}
	m.view.remarks = remarks("三つ目")
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnFinished, Result: usecase.TurnResult{Output: "別の回答"}})
	if want := remarks("三つ目"); m.view.remarks != want {
		t.Errorf("remarks after an answer that no remark starts = %q, want %q", m.view.remarks, want)
	}
	if b.statusFor(s, usecase.SessionEvent{Kind: usecase.SessionTurnStarted, AgentStarted: true}).view.remarks[maxRemarks-1] != "三つ目" {
		t.Error("an agent-started turn dropped the remarks of the status message it took over")
	}
	if b.statusFor(s, usecase.SessionEvent{Kind: usecase.SessionTurnStarted}).view.remarks != ([maxRemarks]string{}) {
		t.Error("a requested turn's new status message carries earlier remarks")
	}
}

// remarks lays out the latest remarks as a status view keeps them, the latest last.
func remarks(rs ...string) [maxRemarks]string {
	var out [maxRemarks]string
	copy(out[maxRemarks-len(rs):], rs)
	return out
}

func TestAgentAnswer(t *testing.T) {
	for _, tc := range []struct {
		name       string
		result     usecase.TurnResult
		background int
		want       answerAction
	}{
		{"done, no background", usecase.TurnResult{Output: "結果"}, 0, postAnswer},
		{"done, background remains", usecase.TurnResult{Output: "待ちます"}, 1, holdAnswer},
		{"done, empty", usecase.TurnResult{Output: " \n"}, 0, dropAnswer},
		{"interrupted, background remains", usecase.TurnResult{Output: "止めました", Interrupted: true}, 1, postAnswer},
		{"interrupted, empty", usecase.TurnResult{Interrupted: true}, 0, dropAnswer},
		{"failed", usecase.TurnResult{Failed: true}, 0, dropAnswer},
	} {
		e := usecase.SessionEvent{Kind: usecase.SessionTurnFinished, AgentStarted: true, Result: tc.result,
			Status: usecase.SessionStatus{BackgroundTasks: tc.background}}
		if got := agentAnswer(e); got != tc.want {
			t.Errorf("%s: agentAnswer = %v, want %v", tc.name, got, tc.want)
		}
	}
}

type failingTokens struct{}

func (failingTokens) Token(context.Context) (string, error) {
	return "", errors.New("no token in tests")
}

// recordingBot is a Bot whose posts outside requests are recorded instead of sent.
func recordingBot(t *testing.T) (*Bot, func() []string) {
	t.Helper()
	b := New("dev", "", failingTokens{}, nil, nil, slog.New(slog.DiscardHandler))
	b.heldGrace = 20 * time.Millisecond
	var mu sync.Mutex
	var posts []string
	b.post = func(_ domain.Session, text string) {
		mu.Lock()
		defer mu.Unlock()
		posts = append(posts, text)
	}
	return b, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(posts)
	}
}

func TestObserveAgentAnswers(t *testing.T) {
	s := domain.Session{ID: "sess", Conversation: domain.Conversation{Platform: "slack", Channel: "C1", Thread: "1.0"}}
	started := func(agent bool) usecase.SessionEvent {
		return usecase.SessionEvent{Kind: usecase.SessionTurnStarted, AgentStarted: agent, Status: usecase.SessionStatus{Running: true}}
	}
	finished := func(output string, background int) usecase.SessionEvent {
		return usecase.SessionEvent{Kind: usecase.SessionTurnFinished, AgentStarted: true,
			Result: usecase.TurnResult{Output: output}, Status: usecase.SessionStatus{BackgroundTasks: background}}
	}
	updated := func(background int) usecase.SessionEvent {
		return usecase.SessionEvent{Kind: usecase.SessionUpdated, Status: usecase.SessionStatus{BackgroundTasks: background}}
	}
	for _, tc := range []struct {
		name   string
		events []usecase.SessionEvent
		want   []string
	}{
		{"the next turn starts after the tasks run out",
			[]usecase.SessionEvent{started(false), started(true), finished("待ちます", 1), updated(0), started(true), finished("終わりました", 0)},
			[]string{"終わりました"}},
		{"the next turn starts before the tasks run out",
			[]usecase.SessionEvent{started(false), started(true), finished("待ちます", 1), started(true),
				{Kind: usecase.SessionTurnUpdated, Status: usecase.SessionStatus{Running: true}}, finished("終わりました", 0)},
			[]string{"終わりました"}},
		{"the tasks run out without a turn",
			[]usecase.SessionEvent{started(false), started(true), finished("最後の結果", 1), updated(1), updated(0)},
			[]string{"最後の結果"}},
		{"a request comes while an answer is held",
			[]usecase.SessionEvent{started(false), started(true), finished("待ちます", 1), started(false), updated(0)},
			nil},
		{"a release meant for an earlier answer",
			[]usecase.SessionEvent{started(false), started(true), finished("A", 1), updated(0), started(true), finished("B", 1)},
			nil},
		{"a later answer released in its own time",
			[]usecase.SessionEvent{started(false), started(true), finished("A", 1), updated(0), started(true), finished("B", 1), updated(0)},
			[]string{"B"}},
		{"an error while background work remains",
			[]usecase.SessionEvent{started(false), started(true), {Kind: usecase.SessionTurnFinished, AgentStarted: true,
				Result: usecase.TurnResult{Output: "API Error", Error: true}, Status: usecase.SessionStatus{BackgroundTasks: 1}}, updated(0)},
			[]string{usecase.TurnErrorText("API Error", false)}},
		{"the next turn stops at the usage limit",
			[]usecase.SessionEvent{started(false), started(true), finished("PR を作りました", 1), updated(0), started(true),
				{Kind: usecase.SessionTurnFinished, AgentStarted: true, Result: usecase.TurnResult{Output: "limit", Error: true, UsageLimited: true}}},
			[]string{usecase.TurnErrorText("limit", true) + "\n\n---\n\nPR を作りました"}},
		{"a turn with a request joined stops at the usage limit",
			[]usecase.SessionEvent{started(false), started(true), finished("PR を作りました", 1), updated(0), started(true),
				{Kind: usecase.SessionTurnFinished, Result: usecase.TurnResult{Output: "limit", Error: true, UsageLimited: true}}},
			[]string{"PR を作りました"}},
		{"the next turn is stopped",
			[]usecase.SessionEvent{started(false), started(true), finished("PR を作りました", 1), started(true),
				{Kind: usecase.SessionTurnFinished, AgentStarted: true, Result: usecase.TurnResult{Interrupted: true}}, updated(0)},
			[]string{"PR を作りました"}},
		{"the next turn ends without an answer while tasks remain",
			[]usecase.SessionEvent{started(false), started(true), finished("PR を作りました", 1), started(true), finished("", 1), updated(0)},
			[]string{"PR を作りました"}},
		{"the next turn fails with the process",
			[]usecase.SessionEvent{started(false), started(true), finished("PR を作りました", 1), updated(0), started(true),
				{Kind: usecase.SessionTurnFinished, AgentStarted: true, Result: usecase.TurnResult{Failed: true}}},
			[]string{"PR を作りました"}},
		{"the next turn ends without an answer",
			[]usecase.SessionEvent{started(false), started(true), finished("PR を作りました", 1), updated(0), started(true), finished("", 0)},
			[]string{"PR を作りました"}},
		{"a request joins the next turn",
			[]usecase.SessionEvent{started(false), started(true), finished("待ちます", 1), updated(0), started(true),
				{Kind: usecase.SessionTurnFinished, Result: usecase.TurnResult{Output: "答え"}}, updated(0)},
			nil},
		{"the control plane stops while an answer is held",
			[]usecase.SessionEvent{started(false), started(true), finished("待ちます", 1), {Kind: usecase.SessionAbandoned, Status: usecase.SessionStatus{BackgroundTasks: 1}}, updated(0)},
			[]string{usecase.ShutdownNotice}},
	} {
		b, posts := recordingBot(t)
		for _, e := range tc.events {
			b.observe(s, e)
		}
		deadline := time.Now().Add(5 * time.Second)
		for b.pendingReleases() > 0 && time.Now().Before(deadline) {
			time.Sleep(b.heldGrace)
		}
		if got := posts(); !slices.Equal(got, tc.want) {
			t.Errorf("%s: posted %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestWaitPostsHeldAnswers(t *testing.T) {
	s := domain.Session{ID: "sess", Conversation: domain.Conversation{Platform: "slack", Channel: "C1", Thread: "1.0"}}
	held := []usecase.SessionEvent{
		{Kind: usecase.SessionTurnStarted},
		{Kind: usecase.SessionTurnStarted, AgentStarted: true},
		{Kind: usecase.SessionTurnFinished, AgentStarted: true, Result: usecase.TurnResult{Output: "最後の結果"},
			Status: usecase.SessionStatus{BackgroundTasks: 1}},
	}
	for _, tc := range []struct {
		name   string
		events []usecase.SessionEvent
		want   []string
	}{
		{"the tasks have run out", append(slices.Clone(held), usecase.SessionEvent{Kind: usecase.SessionUpdated}),
			[]string{"最後の結果"}},
		{"the control plane cuts the tasks short", append(slices.Clone(held),
			usecase.SessionEvent{Kind: usecase.SessionAbandoned, Status: usecase.SessionStatus{BackgroundTasks: 1}},
			usecase.SessionEvent{Kind: usecase.SessionUpdated}),
			[]string{usecase.ShutdownNotice}},
	} {
		b, posts := recordingBot(t)
		b.heldGrace = time.Hour
		for _, e := range tc.events {
			b.observe(s, e)
		}
		b.Wait()
		if got := posts(); !slices.Equal(got, tc.want) {
			t.Errorf("%s: posted %q on shutdown, want %q", tc.name, got, tc.want)
		}
	}
}
