package slackbot

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/slack-go/slack/slackevents"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

func TestRequestMessages(t *testing.T) {
	m := newRequestMessages(time.Hour)
	now := time.Unix(1_700_000_000, 0)
	thread := domain.Conversation{Platform: "slack", Workspace: "T1", Channel: "C1", Thread: "1.0"}

	m.remember(thread, "2.0", now)
	if c, ok := m.find("C1", "2.0", now.Add(time.Minute)); !ok || c != thread {
		t.Errorf("find = %+v, %v; want the request's thread", c, ok)
	}
	if _, ok := m.find("C1", "3.0", now); ok {
		t.Error("found a message that is not a request")
	}
	if _, ok := m.find("C2", "2.0", now); ok {
		t.Error("found the request in another channel")
	}
	if _, ok := m.find("C1", "2.0", now.Add(2*time.Hour)); ok {
		t.Error("the request was remembered past the TTL")
	}
	m.remember(thread, "4.0", now.Add(2*time.Hour))
	if _, ok := m.byTS["C1/2.0"]; ok {
		t.Error("an expired request was kept after remembering another")
	}
}

type stopRecorder struct{ stops []string }

func (s *stopRecorder) Execute(context.Context, usecase.RunAgentInput) (usecase.RunAgentOutput, error) {
	return usecase.RunAgentOutput{}, nil
}

func (s *stopRecorder) StopRequest(_ context.Context, _ domain.AgentName, id domain.SessionID, requestID string) error {
	s.stops = append(s.stops, string(id)+" "+requestID)
	return nil
}

type allowAll struct{}

func (allowAll) Allows(domain.AgentName, domain.Requester) bool { return true }

func TestHandleReactionStopsTheRequest(t *testing.T) {
	turns := &stopRecorder{}
	b := New("dev", "", nil, turns, nil, allowAll{}, slog.New(slog.DiscardHandler))
	thread := domain.Conversation{Platform: platform, Workspace: "T1", Channel: "C1", Thread: "1.0"}
	b.requests.remember(thread, "2.0", time.Now())
	reaction := func(name, itemType, ts, user string) *slackevents.ReactionAddedEvent {
		return &slackevents.ReactionAddedEvent{Reaction: name, User: user, Item: slackevents.Item{Type: itemType, Channel: "C1", Timestamp: ts}}
	}

	b.handleReaction(context.Background(), reaction("thumbsup", "message", "2.0", "U1"), "UBOT", "T1")
	b.handleReaction(context.Background(), reaction(stopReaction, "file", "2.0", "U1"), "UBOT", "T1")
	b.handleReaction(context.Background(), reaction(stopReaction, "message", "9.0", "U1"), "UBOT", "T1")
	b.handleReaction(context.Background(), reaction(stopReaction, "message", "2.0", "UBOT"), "UBOT", "T1")
	if len(turns.stops) != 0 {
		t.Fatalf("stopped %q for reactions that are not a stop on a request by a person", turns.stops)
	}
	b.handleReaction(context.Background(), reaction(stopReaction, "message", "2.0", "U1"), "UBOT", "T1")
	if want := []string{string(thread.SessionID("dev")) + " 2.0"}; !slices.Equal(turns.stops, want) {
		t.Errorf("stopped %q, want %q", turns.stops, want)
	}
}

type cleanRecorder struct {
	mu     sync.Mutex
	cleans []string
	asked  []time.Time
	done   chan struct{}
}

func (c *cleanRecorder) CleanNow(_ context.Context, id domain.SessionID, force bool, _ domain.Requester, askedAt time.Time) error {
	c.mu.Lock()
	c.cleans = append(c.cleans, fmt.Sprintf("%s %v", id, force))
	c.asked = append(c.asked, askedAt)
	c.mu.Unlock()
	c.done <- struct{}{}
	return nil
}

// A clean reaction on the message that started a thread names the thread's session by itself,
// with no memory of the request (docs/adr/app/0046).
func TestHandleReactionCleansTheSession(t *testing.T) {
	cleanup := &cleanRecorder{done: make(chan struct{}, 4)}
	b := New("dev", "", nil, &stopRecorder{}, cleanup, allowAll{}, slog.New(slog.DiscardHandler))
	thread := domain.Conversation{Platform: platform, Workspace: "T1", Channel: "C1", Thread: "1.0"}
	reaction := func(name, user string) *slackevents.ReactionAddedEvent {
		return &slackevents.ReactionAddedEvent{Reaction: name, User: user, EventTimestamp: "1700000000.000100",
			Item: slackevents.Item{Type: "message", Channel: "C1", Timestamp: "1.0"}}
	}

	b.handleReaction(context.Background(), reaction(usecase.CleanReaction, "UBOT"), "UBOT", "T1")
	select {
	case <-cleanup.done:
		t.Fatal("cleaned for the bot's own reaction")
	case <-time.After(200 * time.Millisecond):
	}
	b.handleReaction(context.Background(), reaction(usecase.CleanReaction, "U1"), "UBOT", "T1")
	b.handleReaction(context.Background(), reaction(usecase.ForceCleanReaction, "U1"), "UBOT", "T1")
	for range 2 {
		select {
		case <-cleanup.done:
		case <-time.After(2 * time.Second):
			t.Fatal("the session was not cleaned")
		}
	}
	cleanup.mu.Lock()
	defer cleanup.mu.Unlock()
	id := string(thread.SessionID("dev"))
	slices.Sort(cleanup.cleans)
	if want := []string{id + " false", id + " true"}; !slices.Equal(cleanup.cleans, want) {
		t.Errorf("cleaned %q, want %q", cleanup.cleans, want)
	}
	// When the reaction was put, not when it came: a redelivered one stays as old as it was.
	for _, at := range cleanup.asked {
		if !at.Equal(time.Unix(1700000000, 100000)) {
			t.Errorf("asked at %v, want the reaction's event time", at)
		}
	}
}

func TestSlackTime(t *testing.T) {
	if got := slackTime("1700000000.000100"); !got.Equal(time.Unix(1700000000, 100000)) {
		t.Errorf("slackTime() = %v", got)
	}
	if got := slackTime("bad"); !got.IsZero() {
		t.Errorf("slackTime(bad) = %v, want zero, which no notice precedes", got)
	}
}
