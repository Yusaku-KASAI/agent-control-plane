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

// WorkspaceState is what a session's workspace holds besides what the remote has.
type WorkspaceState struct {
	// Changes is set for uncommitted changes or untracked files.
	Changes bool
	Stash   bool
	// Operation names an operation stopped halfway, one of Operations.
	Operation string
	// Pushed are the branches on origin the workspace pushed to, with the tip it last saw.
	Pushed []BranchTip
	// Local are the local branches, plus an entry named "HEAD" for each worktree in the workspace
	// whose HEAD is detached on commits no branch has.
	Local []LocalBranch
}

// Operations are the operations a workspace may have stopped halfway.
var Operations = []string{"rebase", "merge", "cherry-pick", "revert", "bisect"}

type BranchTip struct {
	Name string
	Tip  string
}

type LocalBranch struct {
	BranchTip
	// Unpushed is set when it holds commits found on no remote-tracking branch and not in the
	// target's checkout the workspace was cloned from.
	Unpushed bool
}

// SessionHolder lets go of a session it holds while it waits idle (Dispatcher.Release).
type SessionHolder interface {
	Release(ctx context.Context, id domain.SessionID) (bool, error)
}

// SessionWorkspace tells what work a session's workspace holds.
type SessionWorkspace interface {
	// State returns nothing for a path that is missing or not a repository. It reads the workspace
	// as the agent's, for the agent may have written anything there.
	State(ctx context.Context, agent domain.Agent, path string) (WorkspaceState, error)
}

// BranchQuery asks about a branch on the remote and where the given commits are kept.
type BranchQuery struct {
	Name    string
	Commits []string
}

// CommitPlaces tells where on the remote a commit is kept.
type CommitPlaces struct {
	InDefault bool
	// InMergedPull and InClosedPull are set when the head of a pull request from the branch, merged
	// or closed without merging, contains the commit.
	InMergedPull bool
	InClosedPull bool
	// InBranch is set when the branch exists and contains the commit.
	InBranch bool
}

type BranchFacts struct {
	Name   string
	Exists bool
	// Tip is set when the branch exists.
	Tip string
	// OpenHead and OpenBase are the open pull requests from and into the branch.
	OpenHead []PullRequest
	OpenBase []PullRequest
	// Commits tells where each queried commit, and Tip, is kept.
	Commits map[string]CommitPlaces
}

// SessionBranches reads and deletes branches on the target's remote. List and Inspect return
// nothing when the agent could not have pushed: without a GitHub token or a GitHub origin.
type SessionBranches interface {
	// List returns the names of the branches on the target's remote that start with prefix.
	List(ctx context.Context, agent domain.Agent, creds domain.Credentials, prefix string) ([]string, error)
	// Inspect tells facts of the branches on the target's remote.
	Inspect(ctx context.Context, agent domain.Agent, creds domain.Credentials, queries []BranchQuery) ([]BranchFacts, error)
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
	// Interrupted is set when the turn was stopped, by Process.Interrupt or otherwise, rather than
	// ended on an error.
	Interrupted bool
	// Failed is set when the process ended before the turn did.
	Failed bool
	// Error is set when the agent ended the turn on an error; Output is then the agent's message
	// about it.
	Error bool
	// UsageLimited is set with Error when the account's usage limit refused the turn's requests.
	UsageLimited bool
	// Usage is the session's total that the agent reported at the end of the turn.
	Usage domain.Usage
	// ContextTokens is the size of the conversation the agent's last API request in the turn read,
	// subagents left out; 0 when the turn made none that reported it.
	ContextTokens int64
}

// TurnRecord is what one finished turn spent, and on whose behalf.
type TurnRecord struct {
	Agent        domain.AgentName
	Session      domain.SessionID
	Conversation domain.Conversation
	StartedAt    time.Time
	FinishedAt   time.Time
	// Requesters asked for the turn. For a turn the agent started by itself, they are those of the
	// work it continues, as for co-authors, or in a new process those of the prompts it holds.
	Requesters   []domain.Requester
	AgentStarted bool
	Result       TurnResult
	// Usage is what the turn spent; nil when it cannot be told from the session's totals.
	Usage *domain.Usage
}

// UsageLog keeps what turns spent.
type UsageLog interface {
	// Record keeps the turn's usage. Recording never fails the turn, so the implementation reports
	// its own failures.
	Record(ctx context.Context, r TurnRecord)
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
	// Activity is what the agent is doing now, for EventActivity; empty when it did not change.
	Activity string
	// Remark is what the agent said to people, for EventActivity; empty when it said nothing.
	Remark string
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
	// Result is set for SessionTurnFinished.
	Result TurnResult
	// AgentStarted is set for SessionTurnStarted when the agent starts the turn by itself, and for
	// SessionTurnFinished when no request waits for the turn's result, which then reaches people
	// only through the observer. A prompt injected into the agent's own turn makes the two differ.
	AgentStarted bool
	// Remark is set for SessionTurnUpdated when the agent said something to people as it works.
	Remark string
	// Spent is set when a turn ended with the agent's result: for SessionTurnFinished, and for the
	// SessionTurnUpdated of an agent's own turn that the running request's turn hides. It is nil for
	// a turn that ended without one, whose usage is unknown.
	Spent *Spent
}

// Spent is what a turn spent and the session's total after it.
type Spent struct {
	// Turn is nil when it cannot be told.
	Turn *domain.Usage
	// Session is the session's total the agent reported, or the last known one when it reported
	// none; nil when it is not known.
	Session *domain.Usage
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
	// their sessions. It removes what was left reading workspaces (SessionWorkspace) too, which
	// holds no session's work.
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
	// Close asks the process to exit, stops it when ctx expires first, and releases it. It returns an
	// error when the process did not exit by itself.
	Close(ctx context.Context) error
}
