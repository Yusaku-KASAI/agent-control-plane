package usecase

import "github.com/Yusaku-KASAI/agent-control-plane/internal/domain"

// InboxSize lets the external tests count what fills the inbox channel.
const InboxSize = inboxSize

// InboxLen tells how many requests wait in the session's inbox channel, for the tests to see a
// request reach it.
func (d *Dispatcher) InboxLen(id domain.SessionID) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if w, ok := d.workers[id]; ok {
		return len(w.inbox)
	}
	return 0
}
