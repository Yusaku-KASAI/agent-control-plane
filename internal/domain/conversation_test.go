package domain_test

import (
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

func TestConversation_SessionID(t *testing.T) {
	thread := domain.Conversation{Platform: "slack", Workspace: "T1", Channel: "C1", Thread: "1700000000.000100"}

	id := thread.SessionID("repo-dev")
	if _, err := domain.ParseSessionID(string(id)); err != nil {
		t.Fatalf("SessionID() = %q is not a valid ID: %v", id, err)
	}
	if string(id)[14] != '5' {
		t.Errorf("SessionID() = %q, want a version 5 UUID", id)
	}
	if again := thread.SessionID("repo-dev"); again != id {
		t.Errorf("SessionID() is not stable: %q then %q", id, again)
	}

	other := thread
	other.Thread = "1700000000.000200"
	for name, got := range map[string]domain.SessionID{
		"another thread": other.SessionID("repo-dev"),
		"another agent":  thread.SessionID("repo-pm"),
	} {
		if got == id {
			t.Errorf("%s got the same session %q", name, id)
		}
	}
}
