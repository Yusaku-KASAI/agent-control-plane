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
	for _, want := range []string{"<@U1>", "channel C1 / ts 12.0", "channel C1 / thread_ts 10.0", "slack スキル", "「書く」の書き方"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt lacks %q:\n%s", want, got)
		}
	}
}

func TestMarkdownText(t *testing.T) {
	if got := markdownText("short"); got != "short" {
		t.Errorf("markdownText(short) = %q", got)
	}
	if got := markdownBlock("PR：https://example.com/pull/1").Text; got != "PR：<https://example.com/pull/1>" {
		t.Errorf("markdownBlock did not fit the text to Slack: %q", got)
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
	if got := running.render(start); !strings.Contains(got, "\nエージェントの直近の発言（古い順）：\n- 原因が分かりました\n\n今の作業：") {
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

func TestStatusForAgentStartedTurn(t *testing.T) {
	b := New("dev", "", nil, nil, nil, nil, 0, slog.New(slog.DiscardHandler))
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
	b := New("dev", "", nil, nil, nil, nil, 0, slog.New(slog.DiscardHandler))
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

type failingTokens struct{}

func (failingTokens) Token(context.Context) (string, error) {
	return "", errors.New("no token in tests")
}

// recordingBot is a Bot whose posts outside requests are recorded instead of sent.
func recordingBot(t *testing.T) (*Bot, func() []string) {
	t.Helper()
	b := New("dev", "", failingTokens{}, nil, nil, nil, 0, slog.New(slog.DiscardHandler))
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

func TestObservePostsNotices(t *testing.T) {
	s := domain.Session{ID: "sess", Conversation: domain.Conversation{Platform: "slack", Channel: "C1", Thread: "1.0"}}
	b, posts := recordingBot(t)
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionAbandoned, Status: usecase.SessionStatus{BackgroundTasks: 1}})
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnFinished, Unrequested: true, Result: usecase.TurnResult{Output: "回答"}})
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionNotice, Notice: "知らせ"})
	if got := posts(); !slices.Equal(got, []string{"知らせ"}) {
		t.Errorf("posted %q, want only the notice", got)
	}
}

func TestStatusViewRenderSpent(t *testing.T) {
	start := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		view  statusView
		want  string
		shown bool
	}{
		{"finished", statusView{outcome: done, spent: spentView{reported: true, costUSD: 0.42, sessionCostUSD: 3.1, sessionKnown: true}}, "費用の見積もり：この依頼 $0.42・セッションの合計 $3.10\n", true},
		{"unknown turn", statusView{outcome: interrupted, spent: spentView{reported: true, unknown: true, sessionCostUSD: 3.1, sessionKnown: true}}, "費用の見積もり：この依頼 不明・セッションの合計 $3.10\n", true},
		{"tiny", statusView{outcome: done, spent: spentView{reported: true, costUSD: 0.004, sessionKnown: true}}, "この依頼 $0.01 未満・セッションの合計 $0.00", true},
		{"unknown total", statusView{outcome: failed, spent: spentView{reported: true, unknown: true}}, "この依頼 不明・セッションの合計 不明", true},
		{"nothing reported", statusView{outcome: failed, spent: spentView{unknown: true}}, "費用", false},
		{"running", statusView{outcome: running, spent: spentView{reported: true, costUSD: 1}}, "費用", false},
	}
	for _, tc := range tests {
		tc.view.session, tc.view.startedAt, tc.view.finishedAt = "sess", start, start
		got := tc.view.render(start)
		if strings.Contains(got, tc.want) != tc.shown {
			t.Errorf("%s: render shows %q = %v, want %v:\n%s", tc.name, tc.want, !tc.shown, tc.shown, got)
		}
	}
}

func TestObserveSumsSpent(t *testing.T) {
	b := New("dev", "", nil, nil, nil, nil, 0, slog.New(slog.DiscardHandler))
	s := domain.Session{ID: "sess", Conversation: domain.Conversation{Platform: "slack", Channel: "C1", Thread: "1.0"}}
	spent := func(turn, session float64) *usecase.Spent {
		return &usecase.Spent{Turn: &domain.Usage{CostUSD: turn}, Session: &domain.Usage{CostUSD: session}}
	}
	m := b.statusFor(s, usecase.SessionEvent{Kind: usecase.SessionTurnStarted})
	m.updating = true // keeps observe from starting to write to Slack

	// The agent's own turn that the request's turn hides counts toward the request.
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnUpdated, Spent: spent(0.25, 1.25), Status: usecase.SessionStatus{Running: true}})
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnUpdated, Remark: "作業中", Status: usecase.SessionStatus{Running: true}})
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnFinished, Spent: spent(0.5, 1.75)})
	if want := (spentView{reported: true, costUSD: 0.75, sessionCostUSD: 1.75, sessionKnown: true}); m.view.spent != want {
		t.Errorf("spent = %+v, want %+v", m.view.spent, want)
	}

	// The agent's own turn takes the message over and adds to it.
	b.statusFor(s, usecase.SessionEvent{Kind: usecase.SessionTurnStarted, AgentStarted: true})
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnFinished, Unrequested: true, Spent: spent(0.25, 2)})
	if want := (spentView{reported: true, costUSD: 1, sessionCostUSD: 2, sessionKnown: true}); m.view.spent != want {
		t.Errorf("spent after the agent's own turn = %+v, want %+v", m.view.spent, want)
	}

	// A turn without the agent's result leaves the request's cost unknown.
	b.statusFor(s, usecase.SessionEvent{Kind: usecase.SessionTurnStarted, AgentStarted: true})
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnFinished, Unrequested: true, Result: usecase.TurnResult{Failed: true}})
	if want := (spentView{reported: true, costUSD: 1, unknown: true, sessionCostUSD: 2, sessionKnown: true}); m.view.spent != want {
		t.Errorf("spent after a failed turn = %+v, want %+v", m.view.spent, want)
	}

	// A turn whose session's total is not known keeps the last one shown.
	b.statusFor(s, usecase.SessionEvent{Kind: usecase.SessionTurnStarted, AgentStarted: true})
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnFinished, Unrequested: true, Spent: &usecase.Spent{}})
	if want := (spentView{reported: true, costUSD: 1, unknown: true, sessionCostUSD: 2, sessionKnown: true}); m.view.spent != want {
		t.Errorf("spent after a turn without a known total = %+v, want %+v", m.view.spent, want)
	}

	if next := b.statusFor(s, usecase.SessionEvent{Kind: usecase.SessionTurnStarted}); next.view.spent != (spentView{}) {
		t.Errorf("a requested turn's new status message carries earlier costs: %+v", next.view.spent)
	}
}

func TestStatusViewRenderClosingSuggestion(t *testing.T) {
	start := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	const suggestion = "💡 コンテキストが約 45 万トークンです。"
	tests := []struct {
		name  string
		view  statusView
		shown bool
	}{
		{"over the threshold", statusView{outcome: done, contextTokens: 456_789, suggestClosingAt: 400_000}, true},
		{"at the threshold after an error", statusView{outcome: failed, contextTokens: 450_000, suggestClosingAt: 450_000}, true},
		{"under the threshold", statusView{outcome: done, contextTokens: 456_789, suggestClosingAt: 500_000}, false},
		{"turned off", statusView{outcome: done, contextTokens: 456_789}, false},
		{"unknown context", statusView{outcome: usageLimited, suggestClosingAt: 400_000}, false},
		{"running", statusView{outcome: running, contextTokens: 456_789, suggestClosingAt: 400_000}, false},
	}
	for _, tc := range tests {
		tc.view.session, tc.view.startedAt, tc.view.finishedAt = "sess", start, start
		got := tc.view.render(start)
		if strings.Contains(got, suggestion) != tc.shown {
			t.Errorf("%s: render shows the suggestion = %v, want %v:\n%s", tc.name, !tc.shown, tc.shown, got)
		}
	}
}

func TestTokenCount(t *testing.T) {
	for n, want := range map[int64]string{8_000: "8000 トークン", 10_000: "1 万トークン", 456_789: "45 万トークン"} {
		if got := tokenCount(n); got != want {
			t.Errorf("tokenCount(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestObserveKeepsContextSize(t *testing.T) {
	b := New("dev", "", nil, nil, nil, nil, 400_000, slog.New(slog.DiscardHandler))
	s := domain.Session{ID: "sess", Conversation: domain.Conversation{Platform: "slack", Channel: "C1", Thread: "1.0"}}
	m := b.statusFor(s, usecase.SessionEvent{Kind: usecase.SessionTurnStarted})
	m.updating = true // keeps observe from starting to write to Slack
	if m.view.suggestClosingAt != 400_000 {
		t.Fatalf("suggestClosingAt = %d, want the bot's", m.view.suggestClosingAt)
	}

	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnFinished, Result: usecase.TurnResult{ContextTokens: 410_000}})
	if m.view.contextTokens != 410_000 {
		t.Errorf("contextTokens = %d, want the turn's", m.view.contextTokens)
	}

	// A turn that made no request keeps the size last known.
	b.statusFor(s, usecase.SessionEvent{Kind: usecase.SessionTurnStarted, AgentStarted: true})
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnFinished, Unrequested: true, Result: usecase.TurnResult{Failed: true}})
	if m.view.contextTokens != 410_000 {
		t.Errorf("contextTokens after a turn without a request = %d, want the last known", m.view.contextTokens)
	}

	// A compacted context goes under the threshold again.
	b.statusFor(s, usecase.SessionEvent{Kind: usecase.SessionTurnStarted, AgentStarted: true})
	b.observe(s, usecase.SessionEvent{Kind: usecase.SessionTurnFinished, Unrequested: true, Result: usecase.TurnResult{ContextTokens: 30_000}})
	if m.view.contextTokens != 30_000 {
		t.Errorf("contextTokens after compacting = %d, want the latest", m.view.contextTokens)
	}
}
