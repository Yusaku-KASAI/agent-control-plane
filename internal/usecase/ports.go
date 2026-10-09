package usecase

import (
	"context"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

type AgentRegistry interface {
	// FindByName returns an error wrapping domain.ErrAgentNotFound when no agent has the name.
	FindByName(ctx context.Context, name domain.AgentName) (domain.Agent, error)
}

type SessionRepository interface {
	// Find returns an error wrapping domain.ErrSessionNotFound when the session does not exist.
	Find(ctx context.Context, id domain.SessionID) (domain.Session, error)
	Save(ctx context.Context, s domain.Session) error
	// Lock returns an error wrapping domain.ErrSessionBusy while someone else holds the session.
	// A session can be locked before it exists.
	Lock(ctx context.Context, id domain.SessionID) (unlock func(), err error)
	// List returns the IDs of every stored session, including those without a record.
	List(ctx context.Context) ([]domain.SessionID, error)
	// Remove deletes the session's record and workspace. The caller holds the lock.
	Remove(ctx context.Context, id domain.SessionID) error
}

type PullRequest struct {
	Number int
	Title  string
	URL    string
}

type BranchState int

const (
	// BranchDone is a branch whose work is kept elsewhere: contained in the default branch or in the
	// head of a closed pull request.
	BranchDone BranchState = iota + 1
	// BranchInReview is the head of an open pull request, or the base of one for an owned branch.
	BranchInReview
	// BranchUnfinished has commits that are in no pull request and not in the default branch.
	BranchUnfinished
)

type LocalBranch struct {
	Name string
	Tip  string
	// Unpushed is set when it holds commits found on no remote-tracking branch and not in the
	// target's checkout the workspace was cloned from.
	Unpushed bool
}

type WorkspaceBranches struct {
	// Pushed are the branches on origin the workspace pushed to, with the tip it last saw.
	Pushed []BranchRef
	// Local are the local branches, plus HEAD named "HEAD" when it is detached on commits no
	// branch has.
	Local []LocalBranch
}

// SessionWorkspace tells what work a session's workspace holds.
type SessionWorkspace interface {
	// Branches returns none for a path that is missing or not a repository.
	Branches(ctx context.Context, path string) (WorkspaceBranches, error)
}

// BranchRef is a branch on the remote, with the tip the workspace has for it.
type BranchRef struct {
	Name     string
	LocalTip string
	// Owned is set for a branch the session created. Only such a branch is deleted, so only its
	// state beyond the pull requests from it matters.
	Owned bool
}

type RemoteBranch struct {
	BranchRef
	State  BranchState
	Exists bool
	// Pulls are the open pull requests from or into the branch.
	Pulls []PullRequest
}

type SessionBranches interface {
	// Inspect tells the state of the branches on the target's remote, leaving out its default
	// branch. A branch gone from the remote is judged by its local tip. A branch not owned that
	// still exists is in review only for pull requests from it, and otherwise done. It returns none when the agent could
	// not have pushed: without a GitHub token or a GitHub origin.
	Inspect(ctx context.Context, agent domain.Agent, creds domain.Credentials, refs []BranchRef) ([]RemoteBranch, error)
	// Remove deletes the named branches on the target's remote and returns those it deleted,
	// leaving out those already gone or protected.
	Remove(ctx context.Context, agent domain.Agent, creds domain.Credentials, names []string) ([]string, error)
}

type SessionNotifier interface {
	// Notify posts text where the session started. It does nothing for a session that did not
	// start in a chat.
	Notify(ctx context.Context, s domain.Session, text string) error
}

type SessionFiles interface {
	// Remove deletes what the runner keeps for the session outside its workspace, such as its
	// credentials and the agent's conversation history.
	Remove(ctx context.Context, agent domain.AgentName, id domain.SessionID) error
}

type WorkspaceProvider interface {
	// Prepare creates the session's workspace from the agent's target, using creds to reach its remote.
	Prepare(ctx context.Context, agent domain.Agent, id domain.SessionID, creds domain.Credentials) (domain.Workspace, error)
}

type People interface {
	// GitIdentity returns how the requester is credited in commits, if known.
	GitIdentity(r domain.Requester) (domain.GitIdentity, bool)
}

type CredentialIssuer interface {
	// Issue returns credentials valid for at least one turn.
	Issue(ctx context.Context, agent domain.Agent) (domain.Credentials, error)
}

type StartRequest struct {
	Agent       domain.Agent
	Session     domain.Session
	Credentials domain.Credentials
}

type TurnResult struct {
	Output string
	// Interrupted is set when the turn ended because of Process.Interrupt.
	Interrupted bool
	// Failed is set when the process ended before the turn did.
	Failed bool
}

type EventKind int

const (
	EventTurnStarted EventKind = iota + 1
	EventTurnFinished
	EventBackgroundTasks
	EventActivity
	// EventPromptAccepted reports that the agent took the prompt into the running turn.
	EventPromptAccepted
)

// ProcessEvent reports a change in the agent process. A turn may start without a prompt,
// when the agent resumes by itself after background work.
type ProcessEvent struct {
	Kind EventKind
	// Result is set for EventTurnFinished.
	Result TurnResult
	// BackgroundTasks lists the IDs of background tasks still running, for EventBackgroundTasks.
	BackgroundTasks []string
	// Activity is what the agent is doing now, for EventActivity.
	Activity string
	// PromptID is set for EventPromptAccepted.
	PromptID string
}

type SessionEventKind int

const (
	SessionTurnStarted SessionEventKind = iota + 1
	SessionTurnUpdated
	SessionTurnFinished
	// SessionUpdated reports a change between turns, such as background tasks ending.
	SessionUpdated
	// SessionAbandoned reports that the control plane stopped the session's process while no
	// request was waiting to be told: on shutdown, with Status showing the work that was cut
	// short, or after a restart, with an empty Status since that work is unknown.
	SessionAbandoned
)

type SessionEvent struct {
	Kind   SessionEventKind
	Status SessionStatus
	// Result and AgentStarted are set for SessionTurnFinished. No request waits for a turn the
	// agent started by itself, so its result reaches people only through the observer.
	Result       TurnResult
	AgentStarted bool
}

type SessionStatus struct {
	Running         bool
	TurnStartedAt   time.Time
	Activity        string
	Queued          int
	BackgroundTasks int
}

// SessionObserver is told how a session's turns progress. Calls come from the session's worker
// in order and must return quickly.
type SessionObserver interface {
	Observe(s domain.Session, e SessionEvent)
}

type Runner interface {
	// Start launches the session's agent process, which then waits for prompts.
	Start(ctx context.Context, r StartRequest) (Process, error)
	// RemoveLeftovers removes the processes a previous control plane left behind and returns
	// their sessions.
	RemoveLeftovers(ctx context.Context) ([]domain.SessionID, error)
}

// Process is one running agent process of a session. Its methods are not called concurrently.
type Process interface {
	// Send delivers a prompt. The agent takes it into the running turn or into a later one,
	// and reports which by EventPromptAccepted with the same id.
	Send(ctx context.Context, id, prompt string) error
	// Interrupt ends the running turn and drops the prompts not yet taken in; background tasks keep running.
	Interrupt(ctx context.Context) error
	StopTask(ctx context.Context, taskID string) error
	// Events is closed when the process exits.
	Events() <-chan ProcessEvent
	// Err reports why the process exited, once Events is closed.
	Err() error
	UpdateCredentials(ctx context.Context, creds domain.Credentials) error
	// SetCoAuthors sets who is credited in commits the agent makes from now on.
	SetCoAuthors(ctx context.Context, people []domain.GitIdentity) error
	// Close asks the process to exit, stops it when ctx expires first, and releases it.
	Close(ctx context.Context) error
}
