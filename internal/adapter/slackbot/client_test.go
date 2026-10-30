package slackbot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slack-go/slack"
)

var quickRetry = webRetry{timeout: 100 * time.Millisecond, retries: 3, initial: time.Millisecond, max: 5 * time.Millisecond}

// A post that got no answer in time, a server error or a rate limit is tried again, so that a short outage does
// not lose the answer (#348); Slack's refusal of the message is not.
func TestWebClientRetries(t *testing.T) {
	for _, tt := range []struct {
		name     string
		failures int
		fail     func(w http.ResponseWriter, stuck <-chan struct{})
		want     int32
		wantErr  bool
	}{
		{"server error", 2, func(w http.ResponseWriter, _ <-chan struct{}) { w.WriteHeader(http.StatusBadGateway) }, 3, false},
		{"no answer in time", 2, func(_ http.ResponseWriter, stuck <-chan struct{}) { <-stuck }, 3, false},
		{"lasting outage", 10, func(w http.ResponseWriter, _ <-chan struct{}) { w.WriteHeader(http.StatusServiceUnavailable) }, 4, true},
		{"rate limited", 1, func(w http.ResponseWriter, _ <-chan struct{}) {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
		}, 2, false},
		{"refused", 10, func(w http.ResponseWriter, _ <-chan struct{}) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_blocks"}`))
		}, 1, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			stuck := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if int(calls.Add(1)) <= tt.failures {
					tt.fail(w, stuck)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true,"channel":"C1","ts":"1700000000.000200"}`))
			}))
			defer srv.Close()
			defer close(stuck)
			api := newWebClient("xoxb-test", "", quickRetry, slack.OptionAPIURL(srv.URL+"/"))

			_, _, err := api.PostMessageContext(context.Background(), "C1", slack.MsgOptionText("answer", false))
			if (err != nil) != tt.wantErr || calls.Load() != tt.want {
				t.Errorf("PostMessage() = %v after %d calls; want error %v after %d", err, calls.Load(), tt.wantErr, tt.want)
			}
		})
	}
}

// A caller that gives up, as putting the cleaned mark does, gets no more attempts: slack-go stops
// retrying when the request's context is done.
func TestWebClientStopsWhenTheCallerGivesUp(t *testing.T) {
	var calls atomic.Int32
	stuck := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
		<-stuck
	}))
	defer srv.Close()
	defer close(stuck)
	api := newWebClient("xoxb-test", "", webRetry{timeout: time.Second, retries: 3, initial: time.Millisecond, max: time.Millisecond}, slack.OptionAPIURL(srv.URL+"/"))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, _, err := api.PostMessageContext(ctx, "C1", slack.MsgOptionText("answer", false)); err == nil || calls.Load() != 1 {
		t.Errorf("PostMessage() = %v after %d calls; want the caller's timeout after 1", err, calls.Load())
	}
}
