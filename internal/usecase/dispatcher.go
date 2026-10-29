package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

var (
	ErrEmptyPrompt   = errors.New("prompt is empty")
	ErrInboxFull     = errors.New("too many requests arrived before the session could take them in")
	ErrShuttingDown  = errors.New("control plane is shutting down")
	ErrStopped       = errors.New("the session was stopped")
	errProcessExited = errors.New("agent process exited")
)

type RunAgentInput struct {
	AgentName domain.AgentName
	// SessionID continues an existing session; empty starts a new one with a random ID.
	SessionID domain.SessionID
	// CreateIfMissing starts a session with SessionID when it does not exist yet.
	CreateIfMissing bool
	Prompt          string
	// Inject sends the prompt into the running turn instead of queueing it behind that turn.
	Inject bool
	// Conversation is recorded on a session this request creates.
	Conversation domain.Conversation
	// Requester is credited in the commits of the turn that answers the prompt.
	Requester domain.Requester
	// RequestID names the request within its session, for StopRequest; empty if it cannot be stopped alone.
	RequestID string
	// Taken, when set, is told when the agent takes the prompt in. Execute calls it on the
	// caller's goroutine before it returns.
	Taken func(Uptake)
}

// Uptake is how the agent took a prompt in.
type Uptake int

const (
	// TakenIntoNewTurn: the prompt began a turn, or the agent took it into a turn it started
	// after the prompt was sent.
	TakenIntoNewTurn Uptake = iota + 1
	// TakenIntoRunningTurn: the prompt was injected into the running turn and the agent took it
	// into that same turn. A turn the agent runs before taking in the prompts sent for a request
	// is part of the request's turn (docs/adr/app/0031), so it does not end the running turn.
	TakenIntoRunningTurn
)

type RunAgentOutput struct {
	Output      string
	Interrupted bool
	// Error and UsageLimited are those of TurnResult.
	Error        bool
	UsageLimited bool
	// Shared is set when an earlier request got the same turn's output, so it need not be repeated.
	Shared bool
	// Session is set whenever it exists, even if the turn then failed.
	Session domain.Session
}

type DispatcherOptions struct {
	// IdleTimeout is how long a session's process waits for the next prompt before it is closed.
	IdleTimeout time.Duration
	// CredentialRefresh is how often credentials of a running process are reissued.
	CredentialRefresh time.Duration
	// StopTimeout bounds a graceful process exit before it is stopped.
	StopTimeout time.Duration
}

// Dispatcher runs each active session in one long-lived agent process and orders its prompts.
type Dispatcher struct {
	agents      AgentRegistry
	sessions    SessionRepository
	workspaces  WorkspaceProvider
	credentials CredentialIssuer
	people      People
	runner      Runner
	observer    SessionObserver
	usage       UsageLog
	opts        DispatcherOptions

	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	workers map[domain.SessionID]*worker
	wg      sync.WaitGroup
}

func NewDispatcher(
	agents AgentRegistry,
	sessions SessionRepository,
	workspaces WorkspaceProvider,
	credentials CredentialIssuer,
	people People,
	runner Runner,
	observer SessionObserver,
	usage UsageLog,
	opts DispatcherOptions,
) *Dispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &Dispatcher{
		agents: agents, sessions: sessions, workspaces: workspaces, credentials: credentials, people: people, runner: runner,
		observer: observer, usage: usage, opts: opts,
		ctx: ctx, cancel: cancel, workers: make(map[domain.SessionID]*worker),
	}
}

// Execute delivers the prompt to the session and waits for the turn that answers it.
func (d *Dispatcher) Execute(ctx context.Context, in RunAgentInput) (RunAgentOutput, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return RunAgentOutput{}, ErrEmptyPrompt
	}
	agent, err := d.agents.FindByName(ctx, in.AgentName)
	if err != nil {
		return RunAgentOutput{}, fmt.Errorf("find agent %q: %w", in.AgentName, err)
	}
	events := make(chan requestEvent, 2)
	r := request{id: in.RequestID, prompt: in.Prompt, inject: in.Inject, requester: in.Requester, events: events}
	session, err := d.openSession(ctx, agent, in)
	if err != nil {
		return RunAgentOutput{}, err
	}
	err = d.submit(ctx, agent, session, r)
	if errors.Is(err, domain.ErrSessionNotFound) && in.CreateIfMissing {
		// The session was cleaned up after openSession found it.
		if session, err = d.openSession(ctx, agent, in); err != nil {
			return RunAgentOutput{}, err
		}
		err = d.submit(ctx, agent, session, r)
	}
	if err != nil {
		return RunAgentOutput{Session: session}, err
	}
	for {
		select {
		case e := <-events:
			if e.reply != nil {
				return e.reply.output(agent.Name)
			}
			if in.Taken != nil {
				in.Taken(e.uptake)
			}
		case <-ctx.Done():
			return RunAgentOutput{Session: session}, ctx.Err()
		}
	}
}

func (r turnReply) output(agent domain.AgentName) (RunAgentOutput, error) {
	if r.err != nil {
		return RunAgentOutput{Session: r.session}, fmt.Errorf("run agent %q: %w", agent, r.err)
	}
	return RunAgentOutput{
		Output: r.result.Output, Interrupted: r.result.Interrupted, Error: r.result.Error, UsageLimited: r.result.UsageLimited,
		Shared: r.shared, Session: r.session,
	}, nil
}

// Stop ends everything the session is doing: the running turn, the background tasks the agent
// would resume after, and the prompts waiting in the queue. A request still in the inbox channel,
// as while the worker starts the agent, may stay and be sent later. It does nothing to an idle
// session.
func (d *Dispatcher) Stop(ctx context.Context, agentName domain.AgentName, id domain.SessionID) error {
	s, err := d.sessions.Find(ctx, id)
	if err != nil {
		return fmt.Errorf("find session %s: %w", id, err)
	}
	if err := s.CheckAgent(agentName); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if w, ok := d.workers[id]; ok {
		select {
		case w.stops <- struct{}{}:
		default:
		}
	}
	return nil
}

// StopRequest stops one request of the session, by how far it has come, as
// docs/architecture/app/requests.md#states tells. The worker takes in the requests still in the inbox
// channel before it looks for the request, so an injected one is sent to the agent if a turn runs.
func (d *Dispatcher) StopRequest(ctx context.Context, agentName domain.AgentName, id domain.SessionID, requestID string) error {
	if requestID == "" {
		return nil
	}
	s, err := d.sessions.Find(ctx, id)
	if err != nil {
		return fmt.Errorf("find session %s: %w", id, err)
	}
	if err := s.CheckAgent(agentName); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if w, ok := d.workers[id]; ok {
		select {
		case w.requestStops <- requestID:
		default:
		}
	}
	return nil
}

// Release retires the session's worker now if it waits idle, holding the session's lock until its
// idle timeout (no turn, no queued request, no background work), so that the session can be
// cleaned at once. It reports whether nothing of the dispatcher holds the session anymore.
func (d *Dispatcher) Release(ctx context.Context, id domain.SessionID) (bool, error) {
	d.mu.Lock()
	w, ok := d.workers[id]
	d.mu.Unlock()
	if !ok {
		return true, nil
	}
	answer := make(chan bool, 1)
	select {
	case w.releases <- answer:
	case <-w.done:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
	select {
	case released := <-answer:
		return released, nil
	case <-w.done:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// Recover removes the agent processes a previous control plane left behind, as when it was
// killed, and tells their sessions. It must run before the first request.
func (d *Dispatcher) Recover(ctx context.Context) (int, error) {
	ids, err := d.runner.RemoveLeftovers(ctx)
	for _, id := range ids {
		s, findErr := d.sessions.Find(ctx, id)
		if findErr != nil {
			err = errors.Join(err, fmt.Errorf("find session %s: %w", id, findErr))
			continue
		}
		d.observer.Observe(s, SessionEvent{Kind: SessionAbandoned})
		if forgetErr := d.forgetUsage(ctx, id); forgetErr != nil {
			err = errors.Join(err, forgetErr)
		}
	}
	return len(ids), err
}

// forgetUsage marks the session's total as unknown: a process removed without exiting may not
// have saved it for the next one to carry over.
func (d *Dispatcher) forgetUsage(ctx context.Context, id domain.SessionID) error {
	unlock, err := d.sessions.Lock(ctx, id)
	if err != nil {
		return fmt.Errorf("lock session %s: %w", id, err)
	}
	defer unlock()
	s, err := d.sessions.Find(ctx, id)
	if err != nil {
		return fmt.Errorf("find session %s: %w", id, err)
	}
	s.ReportedUsage = nil
	if err := d.sessions.Save(ctx, s); err != nil {
		return fmt.Errorf("save session %s: %w", id, err)
	}
	return nil
}

// Close stops every session's process and waits for them.
func (d *Dispatcher) Close() {
	d.cancel()
	d.wg.Wait()
}

func (d *Dispatcher) openSession(ctx context.Context, agent domain.Agent, in RunAgentInput) (domain.Session, error) {
	id := in.SessionID
	if id != "" {
		s, err := d.sessions.Find(ctx, id)
		switch {
		case err == nil:
			if err := s.CheckAgent(agent.Name); err != nil {
				return domain.Session{}, err
			}
			return s, nil
		case !in.CreateIfMissing || !errors.Is(err, domain.ErrSessionNotFound):
			return domain.Session{}, fmt.Errorf("find session %s: %w", id, err)
		}
	} else {
		id = domain.NewRandomSessionID()
	}
	return d.createSession(ctx, agent, id, in.Conversation)
}

// createSession holds the lock while it creates the session, so that SessionCleaner can tell a
// directory being created from one left by a failed creation. A creation that fails removes what it
// left itself.
func (d *Dispatcher) createSession(ctx context.Context, agent domain.Agent, id domain.SessionID, conv domain.Conversation) (domain.Session, error) {
	unlock, err := d.sessions.Lock(ctx, id)
	if err != nil {
		return domain.Session{}, fmt.Errorf("lock session %s: %w", id, err)
	}
	defer unlock()
	if s, err := d.sessions.Find(ctx, id); err == nil {
		return s, s.CheckAgent(agent.Name)
	} else if !errors.Is(err, domain.ErrSessionNotFound) {
		return domain.Session{}, fmt.Errorf("find session %s: %w", id, err)
	}

	s, err := d.makeSession(ctx, agent, id, conv)
	if err != nil {
		// What a failed creation leaves has no record, and a retry with the same ID, as from the same
		// thread, would clone into it and fail. Removing lets the lock go, which is no longer needed.
		if rmErr := d.sessions.Remove(context.WithoutCancel(ctx), id); rmErr != nil {
			err = errors.Join(err, fmt.Errorf("remove the half-made session %s: %w", id, rmErr))
		}
		return domain.Session{}, err
	}
	return s, nil
}

func (d *Dispatcher) makeSession(ctx context.Context, agent domain.Agent, id domain.SessionID, conv domain.Conversation) (domain.Session, error) {
	creds, err := d.credentials.Issue(ctx, agent)
	if err != nil {
		return domain.Session{}, fmt.Errorf("issue credentials for agent %q: %w", agent.Name, err)
	}
	ws, err := d.workspaces.Prepare(ctx, agent, id, creds)
	if err != nil {
		return domain.Session{}, fmt.Errorf("prepare workspace for agent %q: %w", agent.Name, err)
	}
	s := domain.Session{ID: id, AgentName: agent.Name, Workspace: ws, Conversation: conv, LastUsedAt: time.Now()}
	if err := d.sessions.Save(ctx, s); err != nil {
		return domain.Session{}, fmt.Errorf("save session %s: %w", id, err)
	}
	return s, nil
}

const inboxSize = 32

func (d *Dispatcher) submit(ctx context.Context, agent domain.Agent, s domain.Session, r request) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ctx.Err() != nil {
		return ErrShuttingDown
	}
	w, ok := d.workers[s.ID]
	if !ok {
		unlock, err := d.sessions.Lock(ctx, s.ID)
		if err != nil {
			return fmt.Errorf("lock session %s: %w", s.ID, err)
		}
		// SessionCleaner may have removed or replaced the session before the lock was taken.
		current, err := d.sessions.Find(ctx, s.ID)
		if err != nil {
			unlock()
			return fmt.Errorf("find session %s: %w", s.ID, err)
		}
		// Marks the session as used even if no turn finishes.
		current.LastUsedAt = time.Now()
		if err := d.sessions.Save(ctx, current); err != nil {
			unlock()
			return fmt.Errorf("save session %s: %w", s.ID, err)
		}
		s = current
		w = &worker{
			d: d, agent: agent, session: s, unlock: unlock,
			inbox: make(chan request, inboxSize), stops: make(chan struct{}, 1), requestStops: make(chan string, inboxSize),
			releases: make(chan chan bool), done: make(chan struct{}),
		}
		d.workers[s.ID] = w
		d.wg.Go(w.run)
	}
	select {
	case w.inbox <- r:
		return nil
	default:
		return ErrInboxFull
	}
}

// retire removes the worker unless a prompt arrived meanwhile; prompts are only sent under d.mu.
func (d *Dispatcher) retire(w *worker) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(w.inbox) > 0 && d.ctx.Err() == nil {
		return false
	}
	delete(d.workers, w.session.ID)
	return true
}

type request struct {
	id        string
	prompt    string
	inject    bool
	requester domain.Requester
	// events carries, in order, the uptake of the prompt (if the agent takes it in) and then the
	// reply. It holds both, so the worker never blocks on it.
	events chan<- requestEvent
	// injectedInto is the worker's turn the prompt was injected into while it ran; zero if the
	// prompt was not injected into a running turn.
	injectedInto int
}

// requestEvent is either the uptake of a request or its reply.
type requestEvent struct {
	uptake Uptake
	reply  *turnReply
}

// tellTaken tells the requester how the agent took the prompt in; a prompt is taken in at most once.
func (r request) tellTaken(u Uptake) {
	r.events <- requestEvent{uptake: u}
}

// answer replies to the request; a request gets one reply, after its uptake if any.
func (r request) answer(reply turnReply) {
	r.events <- requestEvent{reply: &reply}
}

type turnReply struct {
	result  TurnResult
	shared  bool
	session domain.Session
	err     error
}

// worker owns one session: its process, the requests of its turns, its credentials and what it
// tells the observer. Which request a turn answers is kept by turns, which knows nothing of the
// process.
type worker struct {
	d       *Dispatcher
	agent   domain.Agent
	session domain.Session
	unlock  func()
	inbox   chan request
	stops   chan struct{}
	// requestStops carries the IDs of requests to stop alone.
	requestStops chan string
	// releases asks the worker to retire now if it is idle; it answers on the channel it is
	// given, after giving up the session's lock.
	releases chan chan bool
	// done is closed once the worker has given up the session's lock.
	done chan struct{}

	proc  Process
	turns requestTurns
	// startedAt and activity are those of the running turn; the agent may start one by itself.
	startedAt time.Time
	activity  string
}

func (w *worker) run() {
	var released chan bool
	defer func() {
		w.unlock()
		if released != nil {
			released <- true
		}
		close(w.done)
	}()
	idle := time.NewTimer(w.d.opts.IdleTimeout)
	defer idle.Stop()
	refresh := time.NewTicker(w.d.opts.CredentialRefresh)
	defer refresh.Stop()

	for {
		var events <-chan ProcessEvent
		if w.proc != nil {
			events = w.proc.Events()
		}
		select {
		case r := <-w.inbox:
			w.accept(r)
		case e, ok := <-events:
			if !ok {
				w.processExited()
				break
			}
			w.handle(e)
		case <-w.stops:
			w.stop()
		case id := <-w.requestStops:
			w.stopRequest(id)
		case answer := <-w.releases:
			if w.turns.idle() && w.d.retire(w) {
				released = answer
				w.stopProcess(nil)
				return
			}
			answer <- false
		case <-refresh.C:
			w.refreshCredentials()
		case <-idle.C:
			if w.turns.idle() && w.d.retire(w) {
				w.stopProcess(nil)
				return
			}
		case <-w.d.ctx.Done():
			w.d.retire(w)
			w.shutdown()
			return
		}
		w.sendNext()
		if w.turns.idle() {
			idle.Reset(w.d.opts.IdleTimeout)
		} else {
			idle.Stop()
		}
	}
}

func (w *worker) handle(e ProcessEvent) {
	switch e.Kind {
	case EventTurnStarted:
		if !w.turns.running() {
			w.startTurn(agentTurn)
		}
	case EventTurnFinished:
		w.finishTurn(e.Result)
	case EventBackgroundTasks:
		// A task that ended, stopped or not, may have changed the work, with no turn to tell it.
		if w.turns.setBackground(e.BackgroundTasks) {
			w.markUsed()
		}
		w.notifyStatus()
	case EventActivity:
		if w.turns.running() {
			if e.Activity != "" {
				w.activity = e.Activity
			}
			w.observe(SessionEvent{Kind: SessionTurnUpdated, Remark: e.Remark})
		}
	case EventPromptAccepted:
		r, uptake, started, ok := w.turns.take(e.PromptID)
		if !ok {
			return
		}
		r.tellTaken(uptake)
		if started {
			w.turnStarted(requestsTurn)
		}
		w.creditRequesters()
	}
}

func (w *worker) accept(r request) {
	if r, now := w.turns.arrive(r); now {
		if err := w.send(r); err != nil {
			r.answer(w.reply(TurnResult{}, err))
		}
		return
	}
	if w.turns.running() {
		w.notifyStatus()
	}
}

func (w *worker) sendNext() {
	for {
		r, ok := w.turns.next()
		if !ok {
			return
		}
		if err := w.ensureProcess(); err != nil {
			r.answer(w.reply(TurnResult{}, err))
			continue
		}
		if err := w.send(r); err != nil {
			r.answer(w.reply(TurnResult{}, err))
			w.stopProcess(nil)
			continue
		}
		w.startTurn(requestsTurn)
		return
	}
}

func (w *worker) send(r request) error {
	id := string(domain.NewRandomSessionID())
	if err := w.proc.Send(w.d.ctx, id, r.prompt); err != nil {
		return fmt.Errorf("send prompt: %w", err)
	}
	w.turns.hand(id, r)
	return nil
}

func (w *worker) startTurn(k turnKind) {
	w.turns.start(k)
	w.turnStarted(k)
}

// turnStarted tells of the turn that turns started.
func (w *worker) turnStarted(k turnKind) {
	w.startedAt, w.activity = time.Now(), ""
	w.markUsed()
	w.observe(SessionEvent{Kind: SessionTurnStarted, AgentStarted: k == agentTurn})
}

func (w *worker) notifyStatus() {
	kind := SessionUpdated
	if w.turns.running() {
		kind = SessionTurnUpdated
	}
	w.observe(SessionEvent{Kind: kind})
}

func (w *worker) observe(e SessionEvent) {
	e.Status = SessionStatus{
		Running:         w.turns.running(),
		TurnStartedAt:   w.startedAt,
		Activity:        w.activity,
		Queued:          w.turns.queued(),
		BackgroundTasks: len(w.turns.backgroundTasks()),
	}
	w.d.observer.Observe(w.session, e)
}

// creditRequesters makes the people who asked for the running turn co-authors of its commits.
func (w *worker) creditRequesters() {
	var people []domain.GitIdentity
	for _, r := range w.turns.coAuthors() {
		id, ok := w.d.people.GitIdentity(r)
		if ok && !slices.Contains(people, id) {
			people = append(people, id)
		}
	}
	if err := w.proc.SetCoAuthors(w.d.ctx, people); err != nil {
		w.stopProcess(fmt.Errorf("set co-authors: %w", err))
	}
}

func (w *worker) ensureProcess() error {
	if w.proc != nil {
		return nil
	}
	creds, err := w.d.credentials.Issue(w.d.ctx, w.agent)
	if err != nil {
		return fmt.Errorf("issue credentials: %w", err)
	}
	proc, err := w.d.runner.Start(w.d.ctx, StartRequest{Agent: w.agent, Session: w.session, Credentials: creds})
	if err != nil {
		return fmt.Errorf("start agent process: %w", err)
	}
	w.proc = proc
	return nil
}

func (w *worker) finishTurn(res TurnResult) {
	turnUsage, total := domain.TurnUsage(w.session.UsageBefore(), res.Usage)
	w.session.Turns++
	w.session.LastUsedAt = time.Now()
	w.session.ReportedUsage = total
	err := w.d.sessions.Save(w.d.ctx, w.session)
	if err != nil {
		err = fmt.Errorf("save session: %w", err)
	}
	agentStarted := w.turns.unrequested()
	w.d.usage.Record(w.d.ctx, TurnRecord{
		Agent:        w.agent.Name,
		Session:      w.session.ID,
		Conversation: w.session.Conversation,
		StartedAt:    w.startedAt,
		FinishedAt:   w.session.LastUsedAt,
		Requesters:   w.turns.credit(),
		AgentStarted: agentStarted,
		Result:       res,
		Usage:        turnUsage,
	})
	spent := &Spent{Turn: turnUsage, Session: total}
	end := w.turns.finish()
	if end.hidden {
		w.startedAt, w.activity = time.Now(), ""
		w.observe(SessionEvent{Kind: SessionTurnUpdated, Spent: spent})
		return
	}
	w.observe(SessionEvent{Kind: SessionTurnFinished, Result: res, AgentStarted: agentStarted, Spent: spent})
	for i, r := range end.answered {
		if err != nil {
			r.answer(w.reply(TurnResult{}, err))
			continue
		}
		reply := w.reply(res, nil)
		reply.shared = i > 0
		r.answer(reply)
	}
	if end.next {
		w.startTurn(requestsTurn)
	}
}

// forgetUsage marks the session's total as unknown, for a process that may not have saved it for
// the next one to carry over.
func (w *worker) forgetUsage() error {
	w.session.ReportedUsage = nil
	if err := w.d.sessions.Save(w.d.ctx, w.session); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func (w *worker) stop() {
	for _, r := range w.turns.cancelQueued() {
		r.answer(w.reply(TurnResult{}, ErrStopped))
	}
	w.stopWork()
}

func (w *worker) stopRequest(id string) {
	// A request may still wait in the inbox channel, as while the worker started a process.
	for drained := false; !drained; {
		select {
		case r := <-w.inbox:
			w.accept(r)
		default:
			drained = true
		}
	}
	switch action, r := w.turns.stopRequest(id); action {
	case stopQueued:
		r.answer(w.reply(TurnResult{}, ErrStopped))
		w.notifyStatus()
	case stopAgentWork:
		w.stopWork()
	}
}

// stopWork stops what the agent is doing, leaving the requests waiting in the inbox channel or the queue.
func (w *worker) stopWork() {
	for _, r := range w.turns.cancelSent() {
		r.answer(w.reply(TurnResult{}, ErrStopped))
	}
	if w.proc == nil {
		return
	}
	var err error
	for _, id := range w.turns.backgroundTasks() {
		err = errors.Join(err, w.proc.StopTask(w.d.ctx, id))
	}
	if w.turns.running() {
		err = errors.Join(err, w.proc.Interrupt(w.d.ctx))
	}
	if err != nil {
		w.stopProcess(fmt.Errorf("stop: %w", err))
	}
}

func (w *worker) processExited() {
	err := w.proc.Err()
	if err == nil {
		err = errProcessExited
	}
	if forgetErr := w.forgetUsage(); forgetErr != nil {
		err = errors.Join(err, forgetErr)
	}
	w.stopProcess(err)
}

func (w *worker) refreshCredentials() {
	if w.proc == nil {
		return
	}
	creds, err := w.d.credentials.Issue(w.d.ctx, w.agent)
	if err == nil {
		err = w.proc.UpdateCredentials(w.d.ctx, creds)
	}
	if err != nil {
		w.stopProcess(fmt.Errorf("refresh credentials: %w", err))
	}
}

// markUsed records that the agent may have changed the work now: a turn begins, one ends without
// finishTurn, or a background task ends. SessionCleaner forces only what it told of since (CleanNow). Failing to save leaves
// the time the turn began, or the one before.
func (w *worker) markUsed() {
	w.session.LastUsedAt = time.Now()
	_ = w.d.sessions.Save(context.WithoutCancel(w.d.ctx), w.session)
}

// stopProcess closes the process. With cause, the requests the agent held fail with it.
func (w *worker) stopProcess(cause error) {
	if w.proc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(w.d.ctx), w.d.opts.StopTimeout)
	defer cancel()
	if w.proc.Close(ctx) != nil {
		// The next turn is told as unknown rather than wrong; failing to save only risks the latter.
		_ = w.forgetUsage()
	}
	w.proc = nil
	agentStarted := w.turns.unrequested()
	wasRunning, hadBackground := w.turns.endProcess()
	if wasRunning || hadBackground {
		w.markUsed()
	}
	if cause == nil {
		return
	}
	switch {
	case wasRunning:
		w.observe(SessionEvent{Kind: SessionTurnFinished, Result: TurnResult{Failed: true}, AgentStarted: agentStarted})
	case hadBackground:
		w.observe(SessionEvent{Kind: SessionUpdated})
	}
	for _, r := range w.turns.dropHeld() {
		r.answer(w.reply(TurnResult{}, cause))
	}
}

// shutdown ends the session's process for good. The conversation stays resumable, so a later
// request continues it in a new process.
func (w *worker) shutdown() {
	running := w.turns.running()
	if w.proc != nil && running {
		// Lets the agent record the turn as interrupted before its input closes.
		_ = w.proc.Interrupt(context.WithoutCancel(w.d.ctx))
	}
	waiting := w.turns.waiting() || len(w.inbox) > 0
	if !waiting && (running || len(w.turns.backgroundTasks()) > 0) {
		w.observe(SessionEvent{Kind: SessionAbandoned})
	}
	if running {
		w.turns.endTurn()
		w.markUsed()
		w.observe(SessionEvent{Kind: SessionTurnFinished, Result: TurnResult{Interrupted: true}, AgentStarted: w.turns.unrequested()})
	}
	w.stopProcess(ErrShuttingDown)
	w.failAll(ErrShuttingDown)
}

func (w *worker) failAll(err error) {
	for _, r := range append(w.turns.dropHeld(), w.turns.cancelQueued()...) {
		r.answer(w.reply(TurnResult{}, err))
	}
	for {
		select {
		case r := <-w.inbox:
			r.answer(w.reply(TurnResult{}, err))
		default:
			return
		}
	}
}

func (w *worker) reply(res TurnResult, err error) turnReply {
	return turnReply{result: res, session: w.session, err: err}
}
