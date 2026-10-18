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
	ErrInboxFull     = errors.New("too many requests are waiting for this session")
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
}

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
	reply := make(chan turnReply, 1)
	r := request{prompt: in.Prompt, inject: in.Inject, requester: in.Requester, reply: reply}
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
	select {
	case r := <-reply:
		if r.err != nil {
			return RunAgentOutput{Session: r.session}, fmt.Errorf("run agent %q: %w", agent.Name, r.err)
		}
		return RunAgentOutput{
			Output: r.result.Output, Interrupted: r.result.Interrupted, Error: r.result.Error, UsageLimited: r.result.UsageLimited,
			Shared: r.shared, Session: r.session,
		}, nil
	case <-ctx.Done():
		return RunAgentOutput{Session: session}, ctx.Err()
	}
}

// Stop ends everything the session is doing: the running turn, the background tasks the agent
// would resume after, and the prompts waiting behind them. It does nothing to an idle session.
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
// directory being created from one left by a failed creation.
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
			inbox: make(chan request, inboxSize), stops: make(chan struct{}, 1),
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
	prompt    string
	inject    bool
	requester domain.Requester
	reply     chan<- turnReply
}

type turnReply struct {
	result  TurnResult
	shared  bool
	session domain.Session
	err     error
}

// worker owns one session: its process, its queue and the requests waiting on the running turn.
type worker struct {
	d       *Dispatcher
	agent   domain.Agent
	session domain.Session
	unlock  func()
	inbox   chan request
	stops   chan struct{}

	proc Process
	// running is true from a prompt or a turn start until the turn's result; current may be
	// empty when the agent started the turn by itself. forPrompts tells that the running turn was
	// started for sent prompts rather than by the agent.
	running    bool
	forPrompts bool
	startedAt  time.Time
	activity   string
	background []string
	queue      []request
	// sent holds prompts the agent has not taken in yet, by prompt ID; current holds those taken
	// into the running turn.
	sent    map[string]request
	current []request
	// requesters asked for the latest turn that had requests.
	requesters []domain.Requester
}

func (w *worker) run() {
	defer w.unlock()
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
		case <-refresh.C:
			w.refreshCredentials()
		case <-idle.C:
			if w.idle() && w.d.retire(w) {
				w.stopProcess(nil)
				return
			}
		case <-w.d.ctx.Done():
			w.d.retire(w)
			w.shutdown()
			return
		}
		if !w.running {
			w.sendNext()
		}
		if w.idle() {
			idle.Reset(w.d.opts.IdleTimeout)
		} else {
			idle.Stop()
		}
	}
}

// idle is true when nothing would be lost by closing the process: no turn, no queued prompt,
// and no background task whose completion would start a turn.
func (w *worker) idle() bool {
	return !w.running && len(w.queue) == 0 && len(w.background) == 0
}

func (w *worker) handle(e ProcessEvent) {
	switch e.Kind {
	case EventTurnStarted:
		if !w.running {
			w.startTurn(true)
		}
	case EventTurnFinished:
		w.finishTurn(e.Result)
	case EventBackgroundTasks:
		w.background = e.BackgroundTasks
		w.notifyStatus()
	case EventActivity:
		if w.running {
			if e.Activity != "" {
				w.activity = e.Activity
			}
			w.observe(SessionEvent{Kind: SessionTurnUpdated, Remark: e.Remark})
		}
	case EventPromptAccepted:
		if r, ok := w.sent[e.PromptID]; ok {
			delete(w.sent, e.PromptID)
			w.current = append(w.current, r)
			if !w.running {
				w.startTurn(false)
			}
			w.creditRequesters()
		}
	}
}

func (w *worker) accept(r request) {
	if w.running && r.inject {
		if err := w.send(r); err != nil {
			r.reply <- w.reply(TurnResult{}, err)
		}
		return
	}
	w.queue = append(w.queue, r)
	if w.running {
		w.notifyStatus()
	}
}

func (w *worker) sendNext() {
	for len(w.queue) > 0 {
		r := w.queue[0]
		w.queue = w.queue[1:]
		if err := w.ensureProcess(); err != nil {
			r.reply <- w.reply(TurnResult{}, err)
			continue
		}
		if err := w.send(r); err != nil {
			r.reply <- w.reply(TurnResult{}, err)
			w.stopProcess(nil)
			continue
		}
		w.startTurn(false)
		return
	}
}

func (w *worker) send(r request) error {
	id := string(domain.NewRandomSessionID())
	if err := w.proc.Send(w.d.ctx, id, r.prompt); err != nil {
		return fmt.Errorf("send prompt: %w", err)
	}
	if w.sent == nil {
		w.sent = make(map[string]request)
	}
	w.sent[id] = r
	return nil
}

func (w *worker) startTurn(agentStarted bool) {
	w.running, w.forPrompts, w.startedAt, w.activity = true, !agentStarted, time.Now(), ""
	w.observe(SessionEvent{Kind: SessionTurnStarted, AgentStarted: agentStarted})
}

func (w *worker) notifyStatus() {
	kind := SessionUpdated
	if w.running {
		kind = SessionTurnUpdated
	}
	w.observe(SessionEvent{Kind: kind})
}

func (w *worker) observe(e SessionEvent) {
	e.Status = SessionStatus{
		Running:         w.running,
		TurnStartedAt:   w.startedAt,
		Activity:        w.activity,
		Queued:          len(w.queue),
		BackgroundTasks: len(w.background),
	}
	w.d.observer.Observe(w.session, e)
}

// creditRequesters makes the people who asked for the running turn co-authors of its commits.
// A turn the agent starts by itself keeps crediting the previous requesters, whose work it continues.
func (w *worker) creditRequesters() {
	var people []domain.GitIdentity
	for _, r := range w.current {
		id, ok := w.d.people.GitIdentity(r.requester)
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
	w.recordUsage(res, turnUsage)
	if w.forPrompts && len(w.current) == 0 && len(w.sent) > 0 {
		// The agent ran a turn of its own before taking in the prompts sent for this one, as a
		// resumed session does to report the background tasks its previous process left. The
		// requests' turn is still to come and its answer replaces this one, so it keeps running.
		w.activity = ""
		w.notifyStatus()
		return
	}
	w.running = false
	w.observe(SessionEvent{Kind: SessionTurnFinished, Result: res, AgentStarted: len(w.current) == 0})
	for i, r := range w.current {
		if err != nil {
			r.reply <- w.reply(TurnResult{}, err)
			continue
		}
		reply := w.reply(res, nil)
		reply.shared = i > 0
		r.reply <- reply
	}
	w.current = nil
	if len(w.sent) > 0 {
		// The agent holds prompts it will take into its next turn.
		w.startTurn(false)
	}
}

func (w *worker) recordUsage(res TurnResult, usage *domain.Usage) {
	if len(w.current) > 0 {
		w.requesters = nil
		for _, r := range w.current {
			if !slices.Contains(w.requesters, r.requester) {
				w.requesters = append(w.requesters, r.requester)
			}
		}
	}
	w.d.usage.Record(w.d.ctx, TurnUsage{
		Agent:        w.agent.Name,
		Session:      w.session.ID,
		Conversation: w.session.Conversation,
		StartedAt:    w.startedAt,
		FinishedAt:   w.session.LastUsedAt,
		Requesters:   slices.Clone(w.requesters),
		AgentStarted: len(w.current) == 0,
		Result:       res,
		Usage:        usage,
	})
}

func (w *worker) stop() {
	for _, r := range w.queue {
		r.reply <- w.reply(TurnResult{}, ErrStopped)
	}
	for _, r := range w.sent {
		r.reply <- w.reply(TurnResult{}, ErrStopped)
	}
	w.queue, w.sent = nil, nil
	if w.proc == nil {
		return
	}
	var err error
	for _, id := range w.background {
		err = errors.Join(err, w.proc.StopTask(w.d.ctx, id))
	}
	if w.running {
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
	// A process that ended by itself may not have saved the session's total for the next one.
	w.session.ReportedUsage = nil
	if saveErr := w.d.sessions.Save(w.d.ctx, w.session); saveErr != nil {
		err = errors.Join(err, fmt.Errorf("save session: %w", saveErr))
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

// stopProcess closes the process. With cause, the running turn fails with it.
func (w *worker) stopProcess(cause error) {
	if w.proc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(w.d.ctx), w.d.opts.StopTimeout)
	defer cancel()
	_ = w.proc.Close(ctx)
	w.proc = nil
	wasRunning, hadBackground := w.running, len(w.background) > 0
	w.running, w.background = false, nil
	if cause == nil {
		return
	}
	switch {
	case wasRunning:
		w.observe(SessionEvent{Kind: SessionTurnFinished, Result: TurnResult{Failed: true}, AgentStarted: len(w.current) == 0})
	case hadBackground:
		w.observe(SessionEvent{Kind: SessionUpdated})
	}
	for _, r := range w.current {
		r.reply <- w.reply(TurnResult{}, cause)
	}
	for _, r := range w.sent {
		r.reply <- w.reply(TurnResult{}, cause)
	}
	w.current, w.sent = nil, nil
}

// shutdown ends the session's process for good. The conversation stays resumable, so a later
// request continues it in a new process.
func (w *worker) shutdown() {
	if w.proc != nil && w.running {
		// Lets the agent record the turn as interrupted before its input closes.
		_ = w.proc.Interrupt(context.WithoutCancel(w.d.ctx))
	}
	waiting := len(w.current)+len(w.sent)+len(w.queue)+len(w.inbox) > 0
	if !waiting && (w.running || len(w.background) > 0) {
		w.observe(SessionEvent{Kind: SessionAbandoned})
	}
	if w.running {
		w.running = false
		w.observe(SessionEvent{Kind: SessionTurnFinished, Result: TurnResult{Interrupted: true}, AgentStarted: len(w.current) == 0})
	}
	w.stopProcess(ErrShuttingDown)
	w.failAll(ErrShuttingDown)
}

func (w *worker) failAll(err error) {
	for _, r := range append(w.current, w.queue...) {
		r.reply <- w.reply(TurnResult{}, err)
	}
	w.current, w.queue = nil, nil
	for {
		select {
		case r := <-w.inbox:
			r.reply <- w.reply(TurnResult{}, err)
		default:
			return
		}
	}
}

func (w *worker) reply(res TurnResult, err error) turnReply {
	return turnReply{result: res, session: w.session, err: err}
}
