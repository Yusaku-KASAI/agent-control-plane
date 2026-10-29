package slackbot

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/slack-go/slack"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
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

// The cleaned mark goes on the message that started the thread, and comes off when a session is
// made in it again; Slack's answer that it is already so is no failure (docs/adr/app/0057).
func TestCleanedMark(t *testing.T) {
	type call struct{ method, name, channel, ts string }
	calls := make(chan call, 8)
	var answer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		calls <- call{r.URL.Path, r.Form.Get("name"), r.Form.Get("channel"), r.Form.Get("timestamp")}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	b := New("dev", "", fixedToken("xoxb-test"), &stopRecorder{}, nil, allowAll{}, 0, slog.New(slog.DiscardHandler))
	b.webToken, b.webAPI = "xoxb-test", slack.New("xoxb-test", slack.OptionAPIURL(srv.URL+"/"))
	c := domain.Conversation{Platform: platform, Workspace: "T1", Channel: "C1", Thread: "1700000000.000100"}
	want := func(method string) {
		t.Helper()
		select {
		case got := <-calls:
			if w := (call{method, cleanedReaction, "C1", "1700000000.000100"}); got != w {
				t.Errorf("called %+v, want %+v", got, w)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s was not called", method)
		}
	}

	for _, answer = range []string{`{"ok":true}`, `{"ok":false,"error":"already_reacted"}`} {
		if err := b.MarkCleaned(context.Background(), c); err != nil {
			t.Errorf("MarkCleaned() answered %s = %v, want nil", answer, err)
		}
		want("/reactions.add")
	}
	answer = `{"ok":false,"error":"channel_not_found"}`
	if err := b.MarkCleaned(context.Background(), c); err == nil {
		t.Error("MarkCleaned() = nil on another error")
	}
	want("/reactions.add")

	answer = `{"ok":false,"error":"no_reaction"}`
	b.Observe(domain.Session{ID: "s", Conversation: c}, usecase.SessionEvent{Kind: usecase.SessionCreated})
	want("/reactions.remove")
	if err := b.unmarkCleaned(context.Background(), c); err != nil {
		t.Errorf("unmarkCleaned() with no mark = %v, want nil", err)
	}
	want("/reactions.remove")
	answer = `{"ok":false,"error":"channel_not_found"}`
	if err := b.unmarkCleaned(context.Background(), c); err == nil {
		t.Error("unmarkCleaned() = nil on another error")
	}
	want("/reactions.remove")
}

// Slack not answering, putting the mark gives up, for the cleaner holds the session's lock meanwhile.
func TestMarkCleanedGivesUp(t *testing.T) {
	stuck := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-stuck }))
	defer srv.Close()
	defer close(stuck)
	b := New("dev", "", fixedToken("xoxb-test"), &stopRecorder{}, nil, allowAll{}, 0, slog.New(slog.DiscardHandler))
	b.webToken, b.webAPI = "xoxb-test", slack.New("xoxb-test", slack.OptionAPIURL(srv.URL+"/"))
	b.markTimeout = 50 * time.Millisecond
	done := make(chan error, 1)
	go func() {
		done <- b.MarkCleaned(context.Background(), domain.Conversation{Platform: platform, Channel: "C1", Thread: "1.0"})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("MarkCleaned() = nil, want the timeout")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("MarkCleaned() did not give up")
	}
}
