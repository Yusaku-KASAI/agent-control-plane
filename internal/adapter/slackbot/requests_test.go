package slackbot

import (
	"context"
	"log/slog"
	"slices"
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
	b := New("dev", "", nil, turns, allowAll{}, slog.New(slog.DiscardHandler))
	thread := domain.Conversation{Platform: platform, Workspace: "T1", Channel: "C1", Thread: "1.0"}
	b.requests.remember(thread, "2.0", time.Now())
	reaction := func(name, itemType, ts, user string) *slackevents.ReactionAddedEvent {
		return &slackevents.ReactionAddedEvent{Reaction: name, User: user, Item: slackevents.Item{Type: itemType, Channel: "C1", Timestamp: ts}}
	}

	b.handleReaction(context.Background(), reaction("thumbsup", "message", "2.0", "U1"), "UBOT")
	b.handleReaction(context.Background(), reaction(stopReaction, "file", "2.0", "U1"), "UBOT")
	b.handleReaction(context.Background(), reaction(stopReaction, "message", "9.0", "U1"), "UBOT")
	b.handleReaction(context.Background(), reaction(stopReaction, "message", "2.0", "UBOT"), "UBOT")
	if len(turns.stops) != 0 {
		t.Fatalf("stopped %q for reactions that are not a stop on a request by a person", turns.stops)
	}
	b.handleReaction(context.Background(), reaction(stopReaction, "message", "2.0", "U1"), "UBOT")
	if want := []string{string(thread.SessionID("dev")) + " 2.0"}; !slices.Equal(turns.stops, want) {
		t.Errorf("stopped %q, want %q", turns.stops, want)
	}
}
