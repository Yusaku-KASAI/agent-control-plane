package usecase

import (
	"cmp"
	"maps"
	"slices"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// turnKind is the turn axis of docs/architecture/app/requests.md#states: what started the
// running turn, if one runs.
type turnKind int

const (
	noTurn turnKind = iota
	// requestsTurn was started for sent prompts: on sending one while no turn ran, on the agent
	// taking one in while no turn ran, or on a turn ending while the agent held some.
	requestsTurn
	// agentTurn was started by the agent itself, as when background work ends.
	agentTurn
)

// requestTurns keeps, for one session, which requests wait, which the agent holds and which turn
// answers them, by the axes of docs/architecture/app/requests.md#states. It knows nothing of the
// process: the worker sends the requests it hands out and tells it what the process reports.
type requestTurns struct {
	turn turnKind
	// started counts the turns started, so a request injected into one can tell whether the agent
	// took it into that turn.
	started int
	queue   []request
	// sent holds the requests the agent has not taken in yet, by prompt ID; current holds those
	// taken into the running turn, in the order taken in.
	sent       map[string]request
	current    []request
	background []string
	// requesters are credited with the process's turns that have no requests of their own: those
	// of the latest turn that had requests, or of the requests held when the process began with a
	// turn of its own.
	requesters []domain.Requester
}

func (t *requestTurns) running() bool { return t.turn != noTurn }

// unrequested tells that no request waits for the running turn's result, which then reaches people
// only through the observer (SessionEvent.AgentStarted). It differs from the turn axis: the agent's
// own turn may have taken in an injected request, and a requests' turn may have lost its requests to
// a stop before it ended.
func (t *requestTurns) unrequested() bool { return len(t.current) == 0 }

func (t *requestTurns) queued() int { return len(t.queue) }

// backgroundTasks are the IDs of the background tasks still running.
func (t *requestTurns) backgroundTasks() []string { return t.background }

// idle is true when nothing would be lost by closing the process: no turn, no queued request, and
// no background task whose completion would start a turn.
func (t *requestTurns) idle() bool {
	return !t.running() && len(t.queue) == 0 && len(t.background) == 0
}

// waiting is true when some request still waits to be answered, apart from those not taken out of
// the inbox channel yet.
func (t *requestTurns) waiting() bool {
	return len(t.current)+len(t.sent)+len(t.queue) > 0
}

// workingOnItsOwn tells that the agent works with no request in a turn: in a turn it started by
// itself and took no request into, or on background tasks between turns. A turn the agent runs
// before taking in the requests sent for it belongs to them (docs/adr/app/0031).
func (t *requestTurns) workingOnItsOwn() bool {
	if t.running() {
		return t.turn == agentTurn && len(t.current) == 0
	}
	return len(t.background) > 0
}

// arrive takes in a request taken out of the inbox channel. An injected one is to be sent into the
// running turn now, which it returns marked with that turn; any other waits in the queue.
func (t *requestTurns) arrive(r request) (request, bool) {
	if t.running() && r.inject {
		r.injectedInto = t.started
		return r, true
	}
	t.queue = append(t.queue, r)
	return request{}, false
}

// next hands out the first queued request while no turn runs.
func (t *requestTurns) next() (request, bool) {
	if t.running() || len(t.queue) == 0 {
		return request{}, false
	}
	r := t.queue[0]
	t.queue = t.queue[1:]
	return r, true
}

// hand records a request sent to the agent as the prompt id.
func (t *requestTurns) hand(id string, r request) {
	if t.sent == nil {
		t.sent = make(map[string]request)
	}
	t.sent[id] = r
}

func (t *requestTurns) start(k turnKind) {
	t.started++
	t.turn = k
}

// take moves the request the agent took in as the prompt id into the running turn, starting a
// requests' turn if none runs. ok is false for a prompt that is not held, as one already cancelled.
func (t *requestTurns) take(id string) (r request, u Uptake, started, ok bool) {
	r, ok = t.sent[id]
	if !ok {
		return request{}, 0, false, false
	}
	delete(t.sent, id)
	u = TakenIntoNewTurn
	if t.running() && r.injectedInto != 0 && r.injectedInto == t.started {
		u = TakenIntoRunningTurn
	}
	t.current = append(t.current, r)
	if !t.running() {
		t.start(requestsTurn)
		started = true
	}
	return r, u, started, true
}

// turnEnd is what becomes of a turn the process reported as ended.
type turnEnd struct {
	// hidden is set when the agent ran a turn of its own before taking in the requests sent for
	// the running requests' turn, which keeps running and whose answer replaces this one's
	// (docs/adr/app/0031).
	hidden bool
	// answered are the requests the turn answers, in the order taken in; none for a turn the agent
	// started by itself and took no request into.
	answered []request
	// next is set when the agent still holds requests, which it takes into its next turn: a
	// requests' turn is to start once this one is told.
	next bool
}

func (t *requestTurns) finish() turnEnd {
	if t.turn == requestsTurn && len(t.current) == 0 && len(t.sent) > 0 {
		return turnEnd{hidden: true}
	}
	end := turnEnd{answered: t.current, next: len(t.sent) > 0}
	t.turn, t.current = noTurn, nil
	return end
}

// credit tells who the turn ending now is credited to: the requesters of the requests it took in,
// or for a turn without any, those of the work it continues, or in a new process those of the
// requests the agent holds, which come from the same thread.
func (t *requestTurns) credit() []domain.Requester {
	switch {
	case len(t.current) > 0:
		t.requesters = requesters(t.current)
	case len(t.requesters) == 0:
		t.requesters = requesters(slices.Collect(maps.Values(t.sent)))
	}
	return slices.Clone(t.requesters)
}

// coAuthors are the requesters of the requests taken into the running turn, in the order taken in.
// A turn the agent started by itself has none, and keeps crediting those before.
func (t *requestTurns) coAuthors() []domain.Requester {
	people := make([]domain.Requester, 0, len(t.current))
	for _, r := range t.current {
		people = append(people, r.requester)
	}
	return people
}

// setBackground replaces the background tasks with those still running, and reports whether one
// of them ended.
func (t *requestTurns) setBackground(ids []string) bool {
	ended := slices.ContainsFunc(t.background, func(id string) bool { return !slices.Contains(ids, id) })
	t.background = ids
	return ended
}

// cancelQueued removes the queued requests and returns them.
func (t *requestTurns) cancelQueued() []request {
	rs := t.queue
	t.queue = nil
	return rs
}

// cancelSent removes the requests sent but not taken in and returns them.
func (t *requestTurns) cancelSent() []request {
	rs := slices.Collect(maps.Values(t.sent))
	t.sent = nil
	return rs
}

// stopAction is what stopping one request takes.
type stopAction int

const (
	// stopNothing: the request ended, and the agent works on another request or not at all.
	stopNothing stopAction = iota + 1
	// stopQueued: the request waited in the queue and was removed alone.
	stopQueued
	// stopAgentWork: the agent holds the request, or works with no request, and its work is to stop.
	stopAgentWork
)

// stopRequest tells what stopping the request with the id takes, by how far it has come. A queued
// request is removed and returned.
func (t *requestTurns) stopRequest(id string) (stopAction, request) {
	has := func(r request) bool { return r.id == id }
	if i := slices.IndexFunc(t.queue, has); i >= 0 {
		r := t.queue[i]
		t.queue = slices.Delete(t.queue, i, i+1)
		return stopQueued, r
	}
	if slices.ContainsFunc(t.current, has) || slices.ContainsFunc(slices.Collect(maps.Values(t.sent)), has) || t.workingOnItsOwn() {
		return stopAgentWork, request{}
	}
	return stopNothing, request{}
}

// endTurn forgets the running turn, which ended without the process reporting it.
func (t *requestTurns) endTurn() {
	t.turn = noTurn
}

// endProcess forgets what the process that is gone was doing: its turn, its background tasks and
// whom it credited. It reports whether a turn ran and whether background tasks remained.
func (t *requestTurns) endProcess() (wasRunning, hadBackground bool) {
	wasRunning, hadBackground = t.running(), len(t.background) > 0
	t.turn, t.background, t.requesters = noTurn, nil, nil
	return wasRunning, hadBackground
}

// dropHeld removes the requests the agent held, taken in or not, and returns them.
func (t *requestTurns) dropHeld() []request {
	rs := append(t.current, slices.Collect(maps.Values(t.sent))...)
	t.current, t.sent = nil, nil
	return rs
}

func requesters(rs []request) []domain.Requester {
	var people []domain.Requester
	for _, r := range rs {
		if !slices.Contains(people, r.requester) {
			people = append(people, r.requester)
		}
	}
	// Held requests come in no order; the same people are listed the same way.
	slices.SortFunc(people, func(a, b domain.Requester) int {
		return cmp.Or(cmp.Compare(a.Platform, b.Platform), cmp.Compare(a.ID, b.ID))
	})
	return people
}
