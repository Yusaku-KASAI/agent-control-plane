package slackbot

import (
	"sync"
	"time"
)

// seenMessages remembers recently handled messages. Slack redelivers an event when it misses
// the acknowledgement, and the same mention must not become two requests.
type seenMessages struct {
	mu   sync.Mutex
	ttl  time.Duration
	seen map[string]time.Time
}

func newSeenMessages(ttl time.Duration) *seenMessages {
	return &seenMessages{ttl: ttl, seen: make(map[string]time.Time)}
}

// first reports whether key is new, and remembers it.
func (s *seenMessages) first(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, at := range s.seen {
		if now.Sub(at) > s.ttl {
			delete(s.seen, k)
		}
	}
	if _, ok := s.seen[key]; ok {
		return false
	}
	s.seen[key] = now
	return true
}
