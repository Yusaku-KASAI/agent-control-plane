package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// SessionCleaner removes sessions unused for longer than the retention period and tells their
// threads. It keeps a session holding work found nowhere else or work that may go on (reasons).
type SessionCleaner struct {
	agents      AgentRegistry
	sessions    SessionRepository
	credentials CredentialIssuer
	workspaces  SessionWorkspace
	branches    SessionBranches
	files       SessionFiles
	notifier    SessionNotifier
	retention   time.Duration
	now         func() time.Time
}

type SessionCleanerDeps struct {
	Agents      AgentRegistry
	Sessions    SessionRepository
	Credentials CredentialIssuer
	Workspaces  SessionWorkspace
	Branches    SessionBranches
	Files       SessionFiles
	Notifier    SessionNotifier
}

func NewSessionCleaner(deps SessionCleanerDeps, retention time.Duration, now func() time.Time) *SessionCleaner {
	return &SessionCleaner{
		agents: deps.Agents, sessions: deps.Sessions, credentials: deps.Credentials, workspaces: deps.Workspaces,
		branches: deps.Branches, files: deps.Files, notifier: deps.Notifier, retention: retention, now: now,
	}
}

type CleanReport struct {
	Removed []domain.SessionID
	// Kept lists expired sessions kept for a reason.
	Kept []domain.SessionID
}

var (
	// errKept reports a session left as it is for now, such as one in use.
	errKept = errors.New("session kept")
	// errHasWork reports an expired session kept for a reason.
	errHasWork = errors.New("session holds work")
)

type reasonKind int

const (
	// reasonOpenPull: a branch the session pushed or created is the head of an open pull request,
	// so requests to answer its review may come.
	reasonOpenPull reasonKind = iota + 1
	reasonChanges
	reasonStash
	reasonOperation
	// reasonLostCommits: a branch holds commits the remote keeps nowhere.
	reasonLostCommits
	// reasonUnmerged: a branch the session created, and would delete, is in neither the default
	// branch nor a merged or closed pull request.
	reasonUnmerged
	// reasonOpenBase: deleting a branch the session created would close the pull requests into it.
	reasonOpenBase
	// reasonKindEnd follows the last kind.
	reasonKindEnd
)

// reason is why an expired session is kept: work found nowhere else, or work that may go on.
type reason struct {
	kind      reasonKind
	branch    string
	pull      PullRequest
	operation string
}

// sessionFacts is what the cleaner judges a session by.
type sessionFacts struct {
	workspace WorkspaceState
	// owned are the branches on the remote the session created.
	owned []string
	// branches is nil when the remote cannot be read.
	branches map[string]BranchFacts
}

// reasons is the one place that judges whether a session may be removed. Whether the session
// created a branch decides only whether the branch is deleted, and so whether its own content must
// be kept elsewhere; any commit of the session must be kept somewhere on the remote.
func reasons(f sessionFacts) []reason {
	var rs []reason
	add := func(r reason) {
		if !slices.Contains(rs, r) {
			rs = append(rs, r)
		}
	}
	ws := f.workspace
	if ws.Changes {
		add(reason{kind: reasonChanges})
	}
	if ws.Stash {
		add(reason{kind: reasonStash})
	}
	if ws.Operation != "" {
		add(reason{kind: reasonOperation, operation: ws.Operation})
	}
	kept := func(name, commit string) bool {
		b, ok := f.branches[name]
		p := b.Commits[commit]
		return ok && (p.InDefault || p.InMergedPull || p.InClosedPull || p.InBranch)
	}
	for _, b := range ws.Local {
		if b.Unpushed && (b.Name == "HEAD" || !kept(b.Name, b.Tip)) {
			add(reason{kind: reasonLostCommits, branch: b.Name})
		}
	}
	for _, b := range ws.Pushed {
		if _, ok := f.branches[b.Name]; ok && !kept(b.Name, b.Tip) {
			add(reason{kind: reasonLostCommits, branch: b.Name})
		}
	}
	for _, name := range f.touched() {
		for _, p := range f.branches[name].OpenHead {
			add(reason{kind: reasonOpenPull, pull: p})
		}
	}
	for _, name := range f.owned {
		b := f.branches[name]
		if !b.Exists {
			continue
		}
		// An open pull request from the branch is reason enough; the branch is not in it yet.
		if p := b.Commits[b.Tip]; len(b.OpenHead) == 0 && !p.InDefault && !p.InMergedPull && !p.InClosedPull {
			add(reason{kind: reasonUnmerged, branch: name})
		}
		for _, p := range b.OpenBase {
			add(reason{kind: reasonOpenBase, branch: name, pull: p})
		}
	}
	return rs
}

// touched lists the branches the session pushed or created.
func (f sessionFacts) touched() []string {
	names := slices.Clone(f.owned)
	for _, b := range f.workspace.Pushed {
		if !slices.Contains(names, b.Name) {
			names = append(names, b.Name)
		}
	}
	return names
}

// removable lists the branches to delete: those the session created that still exist.
func (f sessionFacts) removable() []string {
	var names []string
	for _, name := range f.owned {
		if f.branches[name].Exists {
			names = append(names, name)
		}
	}
	return names
}

// Clean removes every expired session that is not in use and has no reason to be kept. It goes on
// past a session it cannot remove, which stays whole enough to be resumed or retried by a later run.
func (c *SessionCleaner) Clean(ctx context.Context) (CleanReport, error) {
	ids, err := c.sessions.List(ctx)
	if err != nil {
		return CleanReport{}, fmt.Errorf("list sessions: %w", err)
	}
	var report CleanReport
	var errs error
	for _, id := range ids {
		if ctx.Err() != nil {
			return report, errors.Join(errs, ctx.Err())
		}
		removed, err := c.clean(ctx, id)
		if removed {
			report.Removed = append(report.Removed, id)
		}
		switch {
		case err == nil, errors.Is(err, errKept), errors.Is(err, domain.ErrSessionBusy):
		case errors.Is(err, errHasWork):
			report.Kept = append(report.Kept, id)
		default:
			errs = errors.Join(errs, fmt.Errorf("session %s: %w", id, err))
		}
	}
	return report, errs
}

// clean looks for reasons to keep the session before taking the lock, so that a kept session is not locked
// against requests while GitHub answers.
func (c *SessionCleaner) clean(ctx context.Context, id domain.SessionID) (removed bool, err error) {
	s, err := c.sessions.Find(ctx, id)
	if errors.Is(err, domain.ErrSessionNotFound) {
		return c.removeUnsaved(ctx, id)
	}
	if err != nil {
		return false, err
	}
	cutoff := c.now().Add(-c.retention)
	if !s.LastUsedAt.Before(cutoff) {
		return false, nil
	}
	agent, err := c.agents.FindByName(ctx, s.AgentName)
	if err != nil {
		return false, fmt.Errorf("find agent %q: %w", s.AgentName, err)
	}
	creds, err := c.credentials.Issue(ctx, agent)
	if err != nil {
		return false, fmt.Errorf("issue credentials for agent %q: %w", agent.Name, err)
	}
	f, err := readFacts(ctx, c.workspaces, c.branches, agent, s.ID.Branch(), s.Workspace.Path, creds)
	if err != nil {
		return false, err
	}
	if rs := reasons(f); len(rs) > 0 {
		if s.KeptNotifiedAt.Before(cutoff) {
			if err := c.notifyKept(ctx, s, rs); err != nil {
				return false, err
			}
		}
		return false, errHasWork
	}
	branches, err := c.remove(ctx, agent, id, creds, cutoff)
	if err != nil {
		return false, err
	}
	if err := c.notifier.Notify(ctx, s, c.removedText(s, branches)); err != nil {
		return true, fmt.Errorf("notify removal: %w", err)
	}
	return true, nil
}

// readFacts reads the workspace and asks the remote about the branches the session pushed or
// created, and about the local branches holding unpushed commits: the remote may keep those commits
// under the same name, as when a merge deleted the branch and a prune removed its remote-tracking
// branch. branch is the session's default working branch, which names those it created.
func readFacts(ctx context.Context, workspaces SessionWorkspace, branches SessionBranches, agent domain.Agent, branch, path string, creds domain.Credentials) (sessionFacts, error) {
	ws, err := workspaces.State(ctx, agent, path)
	if err != nil {
		return sessionFacts{}, fmt.Errorf("read workspace: %w", err)
	}
	f := sessionFacts{workspace: ws}
	listed, err := branches.List(ctx, agent, creds, branch)
	if err != nil {
		return sessionFacts{}, fmt.Errorf("list branches: %w", err)
	}
	f.owned = slices.DeleteFunc(listed, func(name string) bool { return !domain.BranchOwnedBy(branch, name) })
	var queries []BranchQuery
	ask := func(name, commit string) {
		i := slices.IndexFunc(queries, func(q BranchQuery) bool { return q.Name == name })
		if i < 0 {
			queries = append(queries, BranchQuery{Name: name})
			i = len(queries) - 1
		}
		if commit != "" && !slices.Contains(queries[i].Commits, commit) {
			queries[i].Commits = append(queries[i].Commits, commit)
		}
	}
	for _, name := range f.owned {
		ask(name, "")
	}
	for _, b := range ws.Pushed {
		ask(b.Name, b.Tip)
	}
	for _, b := range ws.Local {
		if b.Unpushed && b.Name != "HEAD" {
			ask(b.Name, b.Tip)
		}
	}
	if len(queries) == 0 {
		return f, nil
	}
	facts, err := branches.Inspect(ctx, agent, creds, queries)
	if err != nil {
		return sessionFacts{}, fmt.Errorf("inspect branches: %w", err)
	}
	if facts != nil {
		f.branches = make(map[string]BranchFacts, len(facts))
		for _, b := range facts {
			f.branches[b.Name] = b
		}
	}
	return f, nil
}

// remove deletes the record last: while the record exists the session ID still maps to this
// session, so a new session with the same ID cannot start beside remains of the old one. It looks
// for reasons to keep it again under the lock, as the agent may have pushed or a pull request may have
// been opened meanwhile.
func (c *SessionCleaner) remove(ctx context.Context, agent domain.Agent, id domain.SessionID, creds domain.Credentials, cutoff time.Time) ([]string, error) {
	unlock, err := c.sessions.Lock(ctx, id)
	if err != nil {
		return nil, err
	}
	defer unlock()
	s, err := c.sessions.Find(ctx, id)
	if err != nil {
		return nil, err
	}
	if !s.LastUsedAt.Before(cutoff) {
		return nil, fmt.Errorf("%w: used meanwhile", errKept)
	}
	f, err := readFacts(ctx, c.workspaces, c.branches, agent, s.ID.Branch(), s.Workspace.Path, creds)
	if err != nil {
		return nil, err
	}
	if len(reasons(f)) > 0 {
		return nil, fmt.Errorf("%w: work found meanwhile", errKept)
	}
	var branches []string
	if names := f.removable(); len(names) > 0 {
		if branches, err = c.branches.Remove(ctx, agent, creds, names); err != nil {
			return nil, fmt.Errorf("remove branches: %w", err)
		}
	}
	if err := c.files.Remove(ctx, agent.Name, id); err != nil {
		return nil, fmt.Errorf("remove agent files: %w", err)
	}
	if err := c.sessions.Remove(ctx, id); err != nil {
		return nil, fmt.Errorf("remove session: %w", err)
	}
	return branches, nil
}

// removeUnsaved removes a directory without a record. Sessions are created under the lock, so once
// the lock is taken such a directory is left from a creation that failed or from a request that
// found the session gone.
func (c *SessionCleaner) removeUnsaved(ctx context.Context, id domain.SessionID) (bool, error) {
	unlock, err := c.sessions.Lock(ctx, id)
	if err != nil {
		return false, err
	}
	defer unlock()
	if _, err := c.sessions.Find(ctx, id); !errors.Is(err, domain.ErrSessionNotFound) {
		return false, fmt.Errorf("%w: created meanwhile", errKept)
	}
	return true, c.sessions.Remove(ctx, id)
}

// notifyKept posts without the lock, so that requests need not wait for Slack, and records the
// notice only once it is posted, so that a failed one is retried.
func (c *SessionCleaner) notifyKept(ctx context.Context, s domain.Session, rs []reason) error {
	if err := c.notifier.Notify(ctx, s, c.keptText(s, rs)); err != nil {
		return fmt.Errorf("notify kept session: %w", err)
	}
	return c.update(ctx, s.ID, func(s *domain.Session) { s.KeptNotifiedAt = c.now() })
}

func (c *SessionCleaner) update(ctx context.Context, id domain.SessionID, change func(s *domain.Session)) error {
	unlock, err := c.sessions.Lock(ctx, id)
	if err != nil {
		return err
	}
	defer unlock()
	s, err := c.sessions.Find(ctx, id)
	if err != nil {
		return err
	}
	change(&s)
	return c.sessions.Save(ctx, s)
}

func (c *SessionCleaner) removedText(s domain.Session, branches []string) string {
	removed := "作業領域、会話の履歴"
	for _, b := range branches {
		removed += fmt.Sprintf("、ブランチ `%s`", b)
	}
	return fmt.Sprintf("このスレッドのセッションは、最後に使ってから%sたったため片付けました。\n\n"+
		"消したもの：%s\n\nこのスレッドで続けて依頼すると、前の会話を引き継がない新しいセッションとして始まります。",
		c.since(s), removed)
}

func (c *SessionCleaner) keptText(s domain.Session, rs []reason) string {
	var b strings.Builder
	fmt.Fprintf(&b, "このスレッドのセッションは、最後に使ってから%sたちましたが、次の理由で消さずに残しています。\n", c.since(s))
	for _, r := range rs {
		b.WriteString("\n- ")
		b.WriteString(r.text())
	}
	b.WriteString("\n\nPR をマージするか閉じる、要らない変更やブランチを消すなどで片付くと、次の片付けでセッションを消します。" +
		"続きや片付けを頼む場合は、このスレッドで依頼してください。")
	return b.String()
}

// since tells how long ago the session was last used, in days when at least one.
func (c *SessionCleaner) since(s domain.Session) string {
	const day = 24 * time.Hour
	d := c.now().Sub(s.LastUsedAt)
	if d >= day {
		return fmt.Sprintf(" %d 日", d/day)
	}
	return " " + d.Round(time.Minute).String()
}

// ReasonKinds are the kinds of reasons CheckCleanup tells, which the harness contract publishes.
func ReasonKinds() []string {
	var kinds []string
	for k := reasonOpenPull; k < reasonKindEnd; k++ {
		kinds = append(kinds, reason{kind: k}.key())
		if k == reasonLostCommits {
			kinds = append(kinds, reason{kind: k, branch: "HEAD"}.key())
		}
	}
	return kinds
}

// key names the kind of the reason, for whoever resolves it (CheckCleanup).
func (r reason) key() string {
	switch r.kind {
	case reasonOpenPull:
		return "open-pull"
	case reasonChanges:
		return "changes"
	case reasonStash:
		return "stash"
	case reasonOperation:
		return "operation"
	case reasonLostCommits:
		if r.branch == "HEAD" {
			return "detached-commits"
		}
		return "lost-commits"
	case reasonUnmerged:
		return "unmerged-branch"
	case reasonOpenBase:
		return "open-base"
	}
	return ""
}

func (r reason) text() string {
	switch r.kind {
	case reasonOpenPull:
		return fmt.Sprintf("開いている PR：#%d %s %s", r.pull.Number, r.pull.Title, r.pull.URL)
	case reasonChanges:
		return "作業領域（サブエージェントなどの worktree を含む）に、コミットしていない変更か追跡していないファイルがある"
	case reasonStash:
		return "作業領域の stash に変更がある"
	case reasonOperation:
		return fmt.Sprintf("作業領域（サブエージェントなどの worktree を含む）で %s が途中で止まっている", r.operation)
	case reasonLostCommits:
		if r.branch == "HEAD" {
			return "どのブランチにもないコミットがある（作業領域か、その中のサブエージェントなどの worktree の detached HEAD）"
		}
		return fmt.Sprintf("push していないか、GitHub に見つからないコミットがあるブランチ：`%s`", r.branch)
	case reasonUnmerged:
		return fmt.Sprintf("既定のブランチにも、マージした PR や閉じた PR にも入っていないブランチ：`%s`", r.branch)
	case reasonOpenBase:
		return fmt.Sprintf("`%s` に向けて開いている PR：#%d %s %s", r.branch, r.pull.Number, r.pull.Title, r.pull.URL)
	}
	return ""
}
