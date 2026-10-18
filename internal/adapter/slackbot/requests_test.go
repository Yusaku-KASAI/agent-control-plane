package slackbot

import (
	"testing"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
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
