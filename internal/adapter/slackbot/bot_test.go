package slackbot

import (
	"log/slog"
	"strings"
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

	finished := statusView{session: "sess", startedAt: start, finishedAt: start.Add(9 * time.Second), outcome: done}
	got = finished.render(start.Add(time.Hour))
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

func TestAgentTurnText(t *testing.T) {
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
		{"a turn passes", usecase.SessionEvent{AgentStarted: true, Result: usecase.TurnResult{Output: "ok"}}, "ok"},
		{"the agent's own turn at the limit again", usecase.SessionEvent{AgentStarted: true, Result: limit}, "> You've hit your session limit · resets 2pm"},
		{"the agent's own turn on another error", usecase.SessionEvent{AgentStarted: true, Result: usecase.TurnResult{Output: "API Error", Error: true}}, "エラーでターンが終わりました"},
		{"the agent's own turn failing with the process", usecase.SessionEvent{AgentStarted: true, Result: usecase.TurnResult{Failed: true}}, ""},
		{"the agent's own turn answering", usecase.SessionEvent{AgentStarted: true, Result: usecase.TurnResult{Output: "結果"}}, "結果"},
	}
	for _, st := range steps {
		st.event.Kind = usecase.SessionTurnFinished
		text, ok := b.agentTurnText(s, st.event)
		if ok != (st.want != "") || !strings.Contains(text, st.want) {
			t.Errorf("%s: notice = %q, %v; want one containing %q", st.name, text, ok, st.want)
		}
	}
}
