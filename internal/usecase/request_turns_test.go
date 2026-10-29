package usecase

import (
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// turnsIn builds the state of docs/architecture/app/requests.md#states from its axes.
func turnsIn(turn turnKind, current, sent, queued, background bool) *requestTurns {
	t := &requestTurns{}
	if turn != noTurn {
		t.start(turn)
	}
	if current {
		t.current = []request{{id: "current"}}
	}
	if sent {
		t.hand("p-sent", request{id: "sent"})
	}
	if queued {
		t.queue = []request{{id: "queued"}}
	}
	if background {
		t.background = []string{"task"}
	}
	return t
}

func TestRequestTurns_Arrive(t *testing.T) {
	for _, c := range []struct {
		name   string
		turn   turnKind
		inject bool
		send   bool
	}{
		{"queued with no turn", noTurn, false, false},
		{"injected with no turn waits like any other", noTurn, true, false},
		{"queued behind a requests' turn", requestsTurn, false, false},
		{"injected into a requests' turn", requestsTurn, true, true},
		{"injected into the agent's own turn", agentTurn, true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			turns := turnsIn(c.turn, false, false, false, false)
			r, send := turns.arrive(request{id: "new", inject: c.inject})
			if send != c.send {
				t.Fatalf("send = %v, want %v", send, c.send)
			}
			if send && r.injectedInto != turns.started {
				t.Errorf("injectedInto = %d, want the running turn %d", r.injectedInto, turns.started)
			}
			if queued := len(turns.queue) == 1; queued == send {
				t.Errorf("queued = %v with send = %v", queued, send)
			}
		})
	}
}

func TestRequestTurns_Next(t *testing.T) {
	for _, c := range []struct {
		name string
		turn turnKind
		ok   bool
	}{
		{"no turn hands out the first", noTurn, true},
		{"a requests' turn holds the queue", requestsTurn, false},
		{"the agent's own turn holds the queue", agentTurn, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			turns := turnsIn(c.turn, false, false, true, false)
			r, ok := turns.next()
			if ok != c.ok || (ok && r.id != "queued") {
				t.Errorf("next = %q, %v; want ok %v", r.id, ok, c.ok)
			}
		})
	}
}

func TestRequestTurns_Take(t *testing.T) {
	for _, c := range []struct {
		name    string
		turn    turnKind
		inject  bool
		later   bool // a turn started after the request was injected
		uptake  Uptake
		started bool
	}{
		{"no turn starts a requests' turn", noTurn, false, false, TakenIntoNewTurn, true},
		{"injected into the running turn", agentTurn, true, false, TakenIntoRunningTurn, false},
		{"injected but taken into a later turn", agentTurn, true, true, TakenIntoNewTurn, false},
		{"sent and taken into the turn it began", requestsTurn, false, false, TakenIntoNewTurn, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			turns := turnsIn(c.turn, false, false, false, false)
			r := request{id: "new", inject: c.inject}
			if c.inject {
				r, _ = turns.arrive(r)
			}
			turns.hand("p", r)
			if c.later {
				turns.finish()
				turns.start(requestsTurn)
			}
			_, uptake, started, ok := turns.take("p")
			if !ok || uptake != c.uptake || started != c.started {
				t.Errorf("take = %v, %v, %v; want %v, %v", uptake, started, ok, c.uptake, c.started)
			}
			if len(turns.current) != 1 || len(turns.sent) != 0 {
				t.Errorf("current %d, sent %d; want the request taken in", len(turns.current), len(turns.sent))
			}
			if _, _, _, ok := turns.take("p"); ok {
				t.Error("took the same prompt twice")
			}
		})
	}
}

func TestRequestTurns_Finish(t *testing.T) {
	for _, c := range []struct {
		name          string
		turn          turnKind
		current, sent bool
		hidden        bool
		answered      int
		next          bool
	}{
		{"a requests' turn answers its requests", requestsTurn, true, false, false, 1, false},
		{"the agent's turn before taking in the sent requests is hidden", requestsTurn, false, true, true, 0, false},
		{"a turn leaving sent requests begins the next", requestsTurn, true, true, false, 1, true},
		{"the agent's own turn answers no one", agentTurn, false, false, false, 0, false},
		{"the agent's own turn leaving an injected request begins the next", agentTurn, false, true, false, 0, true},
		{"the agent's own turn with an injected request answers it", agentTurn, true, false, false, 1, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			turns := turnsIn(c.turn, c.current, c.sent, false, false)
			end := turns.finish()
			if end.hidden != c.hidden || len(end.answered) != c.answered || end.next != c.next {
				t.Errorf("finish = %+v; want hidden %v, answered %d, next %v", end, c.hidden, c.answered, c.next)
			}
			if running := turns.running(); running != c.hidden {
				t.Errorf("running = %v after the turn ended", running)
			}
		})
	}
}

func TestRequestTurns_StopRequest(t *testing.T) {
	for _, c := range []struct {
		name                              string
		id                                string
		turn                              turnKind
		current, sent, queued, background bool
		want                              stopAction
	}{
		{"a queued request is removed alone", "queued", requestsTurn, true, false, true, false, stopQueued},
		{"a request taken in stops the work", "current", requestsTurn, true, false, true, false, stopAgentWork},
		{"a request sent but not taken in stops the work", "sent", requestsTurn, false, true, false, false, stopAgentWork},
		{"an ended request leaves another request's turn", "ended", requestsTurn, true, false, false, true, stopNothing},
		{"an ended request leaves the turn before the sent requests", "ended", requestsTurn, false, true, false, false, stopNothing},
		{"an ended request stops the agent's own turn", "ended", agentTurn, false, false, false, false, stopAgentWork},
		{"an ended request leaves the agent's turn holding an injected one", "ended", agentTurn, true, false, false, false, stopNothing},
		{"an ended request stops background work between turns", "ended", noTurn, false, false, false, true, stopAgentWork},
		{"an ended request with nothing running does nothing", "ended", noTurn, false, false, true, false, stopNothing},
	} {
		t.Run(c.name, func(t *testing.T) {
			turns := turnsIn(c.turn, c.current, c.sent, c.queued, c.background)
			action, r := turns.stopRequest(c.id)
			if action != c.want {
				t.Fatalf("stopRequest = %v, want %v", action, c.want)
			}
			if action == stopQueued && (r.id != c.id || len(turns.queue) != 0) {
				t.Errorf("removed %q, %d left in the queue", r.id, len(turns.queue))
			}
		})
	}
}

func TestRequestTurns_Idle(t *testing.T) {
	for _, c := range []struct {
		name               string
		turn               turnKind
		queued, background bool
		want               bool
	}{
		{"nothing", noTurn, false, false, true},
		{"a turn", agentTurn, false, false, false},
		{"a queued request", noTurn, true, false, false},
		{"background tasks", noTurn, false, true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := turnsIn(c.turn, false, false, c.queued, c.background).idle(); got != c.want {
				t.Errorf("idle = %v, want %v", got, c.want)
			}
		})
	}
}

func TestRequestTurns_Credit(t *testing.T) {
	alice, bob := domain.Requester{Platform: "slack", ID: "A"}, domain.Requester{Platform: "slack", ID: "B"}
	turns := &requestTurns{}
	turns.start(requestsTurn)
	turns.hand("p", request{requester: bob})
	if got := turns.credit(); len(got) != 1 || got[0] != bob {
		t.Fatalf("a new process's own turn credits %v, want the held request's %v", got, bob)
	}
	turns.take("p")
	turns.current = append(turns.current, request{requester: alice})
	if got := turns.credit(); len(got) != 2 || got[0] != alice || got[1] != bob {
		t.Fatalf("credit = %v, want the turn's requesters, sorted", got)
	}
	turns.finish()
	turns.start(agentTurn)
	if got := turns.credit(); len(got) != 2 {
		t.Errorf("the agent's own turn credits %v, want those of the work it continues", got)
	}
}

func TestRequestTurns_EndProcess(t *testing.T) {
	for _, c := range []struct {
		name                      string
		turn                      turnKind
		current, sent, background bool
		wasRunning, hadBackground bool
		held                      int
	}{
		{"a turn holding requests taken in and sent", requestsTurn, true, true, false, true, false, 2},
		{"a turn with only sent requests", requestsTurn, false, true, false, true, false, 1},
		{"background tasks between turns", noTurn, false, false, true, false, true, 0},
		{"nothing", noTurn, false, false, false, false, false, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			turns := turnsIn(c.turn, c.current, c.sent, true, c.background)
			turns.requesters = []domain.Requester{{Platform: "slack", ID: "A"}}
			wasRunning, hadBackground := turns.endProcess()
			if wasRunning != c.wasRunning || hadBackground != c.hadBackground {
				t.Errorf("endProcess = %v, %v; want %v, %v", wasRunning, hadBackground, c.wasRunning, c.hadBackground)
			}
			if turns.running() || len(turns.backgroundTasks()) > 0 || len(turns.requesters) > 0 {
				t.Error("the turn, the background tasks or the requesters outlived the process")
			}
			if held := turns.dropHeld(); len(held) != c.held {
				t.Errorf("dropHeld returned %d, want %d", len(held), c.held)
			}
			if turns.waiting() != true || turns.queued() != 1 {
				t.Error("the queued request did not stay")
			}
		})
	}
}

func TestRequestTurns_Waiting(t *testing.T) {
	for _, c := range []struct {
		name                  string
		current, sent, queued bool
		want                  bool
	}{
		{"nothing", false, false, false, false},
		{"taken in", true, false, false, true},
		{"sent", false, true, false, true},
		{"queued", false, false, true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := turnsIn(requestsTurn, c.current, c.sent, c.queued, false).waiting(); got != c.want {
				t.Errorf("waiting = %v, want %v", got, c.want)
			}
		})
	}
}

func TestRequestTurns_Cancel(t *testing.T) {
	turns := turnsIn(requestsTurn, true, true, true, false)
	if rs := turns.cancelQueued(); len(rs) != 1 || rs[0].id != "queued" {
		t.Errorf("cancelQueued = %v", rs)
	}
	if rs := turns.cancelSent(); len(rs) != 1 || rs[0].id != "sent" {
		t.Errorf("cancelSent = %v", rs)
	}
	if turns.queued() != 0 || len(turns.sent) != 0 || len(turns.current) != 1 {
		t.Error("cancelling took more or less than the queue and the sent requests")
	}
	if turns.unrequested() {
		t.Error("the request taken in was cancelled")
	}
}

func TestRequestTurns_SetBackground(t *testing.T) {
	for _, c := range []struct {
		name        string
		before, now []string
		ended       bool
	}{
		{"a task starts", nil, []string{"a"}, false},
		{"a task ends", []string{"a", "b"}, []string{"b"}, true},
		{"one ends as another starts", []string{"a"}, []string{"b"}, true},
		{"nothing changes", []string{"a"}, []string{"a"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			turns := &requestTurns{background: c.before}
			if ended := turns.setBackground(c.now); ended != c.ended {
				t.Errorf("ended = %v, want %v", ended, c.ended)
			}
			if len(turns.backgroundTasks()) != len(c.now) {
				t.Errorf("background = %v, want %v", turns.backgroundTasks(), c.now)
			}
		})
	}
}

func TestRequestTurns_AgentStarts(t *testing.T) {
	for _, c := range []struct {
		name  string
		turn  turnKind
		began bool
	}{
		{"no turn begins the agent's own", noTurn, true},
		{"a requests' turn stays", requestsTurn, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			turns := turnsIn(c.turn, false, false, false, false)
			if began := turns.agentStarts(); began != c.began {
				t.Errorf("agentStarts = %v, want %v", began, c.began)
			}
			if want := map[bool]turnKind{true: agentTurn, false: c.turn}[c.began]; turns.turn != want {
				t.Errorf("turn = %v, want %v", turns.turn, want)
			}
		})
	}
}
