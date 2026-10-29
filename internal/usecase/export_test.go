package usecase

import "github.com/Yusaku-KASAI/agent-control-plane/internal/domain"

// Arrivals tells how many requests arrived for the session that its worker has not taken yet, for
// the tests to see a request reach it.
func (d *Dispatcher) Arrivals(id domain.SessionID) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if w, ok := d.workers[id]; ok {
		return len(w.arrivals)
	}
	return 0
}
