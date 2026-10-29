package slackbot

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/slack-go/slack"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

type fixedToken string

func (t fixedToken) Token(context.Context) (string, error) { return string(t), nil }

// PostMessage returns the ts Slack gives the posted message: the cleaner tells a :fire: on a
// notice by it (docs/adr/app/0056).
func TestPostMessageReturnsTheMessagesTS(t *testing.T) {
	var thread string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat.postMessage" {
			http.NotFound(w, r)
			return
		}
		_ = r.ParseForm()
		thread = r.Form.Get("thread_ts")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"channel":"C1","ts":"1700000000.000200"}`))
	}))
	defer srv.Close()
	b := New("dev", "", fixedToken("xoxb-test"), &stopRecorder{}, nil, allowAll{}, 0, slog.New(slog.DiscardHandler))
	b.webToken, b.webAPI = "xoxb-test", slack.New("xoxb-test", slack.OptionAPIURL(srv.URL+"/"))

	ts, err := b.PostMessage(context.Background(), domain.Conversation{Platform: platform, Channel: "C1", Thread: "1700000000.000100"}, "kept")
	if err != nil || ts != "1700000000.000200" {
		t.Errorf("PostMessage() = %q, %v; want the posted message's ts", ts, err)
	}
	if thread != "1700000000.000100" {
		t.Errorf("posted to thread %q, want the conversation's", thread)
	}
}
