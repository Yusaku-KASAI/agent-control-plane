package slackbot

import (
	"strings"
	"testing"

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
				Text:         "fix the bug",
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
				Text:         "and  again",
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
