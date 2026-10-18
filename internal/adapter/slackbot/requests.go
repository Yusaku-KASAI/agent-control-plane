package slackbot

import (
	"sync"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// requestMemory is how long a request message is remembered for stop reactions. It outlasts any
// request and the agent's work after it; the memory is lost on restart.
const requestMemory = 24 * time.Hour

// requestMessages remembers the conversation each request message belongs to. A reaction tells
// only the message, not its thread, and the thread decides the session.
type requestMessages struct {
	mu   sync.Mutex
	ttl  time.Duration
	byTS map[string]rememberedRequest
}

type rememberedRequest struct {
	conv domain.Conversation
	at   time.Time
}

func newRequestMessages(ttl time.Duration) *requestMessages {
	return &requestMessages{ttl: ttl, byTS: make(map[string]rememberedRequest)}
}

func (m *requestMessages) remember(c domain.Conversation, ts string, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, r := range m.byTS {
		if now.Sub(r.at) > m.ttl {
			delete(m.byTS, k)
		}
	}
	m.byTS[c.Channel+"/"+ts] = rememberedRequest{conv: c, at: now}
}

// find gives the conversation of the request message at channel and ts, if it is remembered.
func (m *requestMessages) find(channel, ts string, now time.Time) (domain.Conversation, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.byTS[channel+"/"+ts]
	if !ok || now.Sub(r.at) > m.ttl {
		return domain.Conversation{}, false
	}
	return r.conv, true
}
