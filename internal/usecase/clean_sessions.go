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

// SessionCleaner removes sessions unused for longer than the retention period, or one a person
// asks to clean now (CleanNow), and tells their threads. It keeps a session holding work found nowhere else or work that may go on (reasons).
type SessionCleaner struct {
	agents      AgentRegistry
	sessions    SessionRepository
	credentials CredentialIssuer
	workspaces  SessionWorkspace
	branches    SessionBranches
	files       SessionFiles
	notifier    SessionNotifier
	holder      SessionHolder
	retention   time.Duration
	// cleaning runs the cleanings of one session one at a time, so that a later one finds what
	// an earlier one did: removed, or kept and told.
	cleaningMu sync.Mutex
	cleaning   map[domain.SessionID]*cleaningLock
	// told holds, for a while, the reasons each person was told on asking to clean a session now,
	// so that their :fire: lets go of these alone. It is not in the session's record, whose lock
	// an agent waiting idle holds; lost on a restart, :wastebasket: is put again.
	toldMu sync.Mutex
	told   map[domain.SessionID]notice
	now    func() time.Time
}

// notice is the reasons a person was told on their :wastebasket:, and when: on the control
// plane's clock (at) and on the chat's (chatAt), when they asked plus the time the judgement took.
// usedAt is the session's LastUsedAt read before the reasons were: once it moves, the work may
// hold what they were not told of.
type notice struct {
	by      domain.Requester
	at      time.Time
	chatAt  time.Time
	usedAt  time.Time
	reasons []string
}

type SessionCleanerDeps struct {
	// Holder lets go of the sessions whose agents wait idle, for cleaning them now; nil holds none.
	Holder      SessionHolder
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
		branches: deps.Branches, files: deps.Files, notifier: deps.Notifier, holder: deps.Holder, retention: retention, now: now,
		cleaning: make(map[domain.SessionID]*cleaningLock), told: make(map[domain.SessionID]notice),
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
	// reasonLostCommits: a branch holds commits never pushed that the remote keeps nowhere.
	reasonLostCommits
	// reasonUnmerged: a branch the session created, and would delete, is in neither the default
	// branch nor a merged or closed pull request.
	reasonUnmerged
	// reasonOpenBase: deleting a branch the session created would close the pull requests into it.
	reasonOpenBase
	// reasonLostPush: the commit the session last pushed to a branch is no longer on the remote,
	// as when another writer force-pushed; only the workspace keeps it.
	reasonLostPush
	// reasonKindEnd follows the last kind.
	reasonKindEnd
)

// reason is why an expired session is kept: work found nowhere else, or work that may go on.
type reason struct {
	kind   reasonKind
	branch string
	// tip is the head of a branch to be deleted (reasonUnmerged), which others may push to.
	tip       string
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
	lostPush := func(b BranchTip) bool {
		_, ok := f.branches[b.Name]
		return ok && !kept(b.Name, b.Tip)
	}
	for _, b := range ws.Local {
		if !b.Unpushed || b.Name != "HEAD" && kept(b.Name, b.Tip) {
			continue
		}
		// A branch at a commit pushed, under any name, which others pushed away, holds nothing never
		// pushed: that is the lost push alone. Without the remote, the lost push is unseen, so the
		// commits stay the workspace's.
		if slices.ContainsFunc(ws.Pushed, func(p BranchTip) bool { return p.Tip == b.Tip && lostPush(p) }) {
			continue
		}
		add(reason{kind: reasonLostCommits, branch: b.Name})
	}
	for _, b := range ws.Pushed {
		if lostPush(b) {
			add(reason{kind: reasonLostPush, branch: b.Name})
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
			add(reason{kind: reasonUnmerged, branch: name, tip: b.Tip})
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
	defer c.cleanOne(id)()
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
	if rs := reasons(f); keeps(rs, false) {
		if s.KeptNotifiedAt.Before(cutoff) {
			if err := c.notifyKept(ctx, s, rs); err != nil {
				return false, err
			}
		}
		return false, errHasWork
	}
	branches, _, err := c.remove(ctx, agent, id, creds, func(s domain.Session) bool { return s.LastUsedAt.Before(cutoff) }, false, nil)
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
func (c *SessionCleaner) remove(ctx context.Context, agent domain.Agent, id domain.SessionID, creds domain.Credentials, unused func(domain.Session) bool, force bool, told []string) (branches []string, discarded []reason, err error) {
	unlock, err := c.sessions.Lock(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	defer unlock()
	s, err := c.sessions.Find(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if !unused(s) {
		return nil, nil, fmt.Errorf("%w: used meanwhile", errKept)
	}
	f, err := readFacts(ctx, c.workspaces, c.branches, agent, s.ID.Branch(), s.Workspace.Path, creds)
	if err != nil {
		return nil, nil, err
	}
	// Forced, it lets go only of what the thread was told.
	rs := reasons(f)
	if keeps(rs, force) || (force && slices.ContainsFunc(rs, func(r reason) bool { return !slices.Contains(told, r.name()) })) {
		return nil, nil, workFound{rs}
	}
	if names := f.removable(); len(names) > 0 {
		if branches, err = c.branches.Remove(ctx, agent, creds, names); err != nil {
			return nil, nil, fmt.Errorf("remove branches: %w", err)
		}
	}
	if err := c.files.Remove(ctx, agent.Name, id); err != nil {
		return nil, nil, fmt.Errorf("remove agent files: %w", err)
	}
	if err := c.sessions.Remove(ctx, id); err != nil {
		return nil, nil, fmt.Errorf("remove session: %w", err)
	}
	return branches, rs, nil
}

// removeUnsaved removes a directory without a record. Sessions are created under the lock, so once
// the lock is taken such a directory is left from a creation that crashed or could not remove what
// it left, or from a request that found the session gone.
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
	c.setTold(s.ID, nil)
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
		"続きを頼む場合は、このスレッドで依頼してください。" + cleanHint)
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
	case reasonLostPush:
		return "lost-push"
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
		return fmt.Sprintf("push していないコミットがあるブランチ：`%s`", r.branch)
	case reasonUnmerged:
		return fmt.Sprintf("既定のブランチにも、マージした PR や閉じた PR にも入っていないブランチ：`%s`", r.branch)
	case reasonOpenBase:
		return fmt.Sprintf("`%s` に向けて開いている PR：#%d %s %s", r.branch, r.pull.Number, r.pull.Title, r.pull.URL)
	case reasonLostPush:
		return fmt.Sprintf("最後に push したコミットが、リモートのブランチにも既定のブランチにも PR にも見つからないブランチ（ほかの書き手の force push など）：`%s`", r.branch)
	}
	return ""
}

// cleaningLock lets one cleaning of a session run at a time; users counts those waiting or running,
// so that it goes once none is.
type cleaningLock struct {
	sync.Mutex
	users int
}

// cleanOne waits for the other cleanings of the session in this process and returns the release.
func (c *SessionCleaner) cleanOne(id domain.SessionID) func() {
	c.cleaningMu.Lock()
	l, ok := c.cleaning[id]
	if !ok {
		l = &cleaningLock{}
		c.cleaning[id] = l
	}
	l.users++
	c.cleaningMu.Unlock()
	l.Lock()
	return func() {
		l.Unlock()
		c.cleaningMu.Lock()
		defer c.cleaningMu.Unlock()
		if l.users--; l.users == 0 {
			delete(c.cleaning, id)
		}
	}
}

// workFound is a session kept for the reasons the judgement under the lock found.
type workFound struct{ reasons []reason }

func (workFound) Error() string { return "session kept: work found meanwhile" }
func (workFound) Unwrap() error { return errKept }

// readingTime is how long after the thread is told a person is taken to have read it: it also
// covers the time the reaction took to reach the control plane, which the notice's time leaves out.
const readingTime = 10 * time.Second

// forceWindow is how long after the thread is told the person told may still force: a :fire: put
// days after, by habit, is not taken as having just weighed the reasons.
const forceWindow = time.Hour

// name tells the reason apart from others: its kind and what it is about.
func (r reason) name() string {
	return fmt.Sprintf("%s %s %s %d %s", r.key(), r.branch, r.tip, r.pull.Number, r.operation)
}

func names(rs []reason) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.name())
	}
	return out
}

// keeps tells whether the reasons keep the session. Any reason does, unless a person forces the
// removal, accepting to lose the work found nowhere else; a pull request open from or into the
// session's branches keeps it even then, for removing would close it or leave its review without
// the session.
func keeps(rs []reason, force bool) bool {
	if !force {
		return len(rs) > 0
	}
	return slices.ContainsFunc(rs, blocksForce)
}

func blocksForce(r reason) bool {
	return r.kind == reasonOpenPull || r.kind == reasonOpenBase
}

// CleanNow judges one session now, whatever its retention, as a person asked in its thread at
// askedAt, on the chat's clock, and tells the thread what came of it: removed, kept and why, or
// not judged. The judgement reads without the lock; only a session to remove has an agent waiting
// idle let go of, and one at work keeps it. With force, the work found nowhere else does not keep
// it (keeps), once that person was told of it on their own request, a while before askedAt, with
// the session not used since its work was read for the notice: forcing before only tells. What the judgement under the lock
// finds beyond what was told keeps the session too.
func (c *SessionCleaner) CleanNow(ctx context.Context, id domain.SessionID, force bool, by domain.Requester, askedAt time.Time) error {
	q := cleanRequest{by: by, at: askedAt, received: c.now()}
	defer c.cleanOne(id)()
	s, err := c.sessions.Find(ctx, id)
	if errors.Is(err, domain.ErrSessionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	err = c.cleanNow(ctx, s, force, q)
	if err == nil || errors.Is(err, errNotTold) {
		return err
	}
	// Told nothing, the person could not tell a failure from a judgement still running.
	return errors.Join(err, c.notifier.Notify(ctx, s, "このスレッドのセッションの片付けを判定できませんでした。"+
		"しばらくしてから、リアクションを外して付け直してください。"))
}

// cleanRequest is a person's asking to clean now: by whom, when on the chat's clock, and when the
// control plane received it on its own.
type cleanRequest struct {
	by       domain.Requester
	at       time.Time
	received time.Time
}

// errNotTold is a failure to post to the thread, which posting again would not mend.
var errNotTold = errors.New("thread not told")

func (c *SessionCleaner) cleanNow(ctx context.Context, s domain.Session, force bool, q cleanRequest) error {
	agent, err := c.agents.FindByName(ctx, s.AgentName)
	if err != nil {
		return fmt.Errorf("find agent %q: %w", s.AgentName, err)
	}
	creds, err := c.credentials.Issue(ctx, agent)
	if err != nil {
		return fmt.Errorf("issue credentials for agent %q: %w", agent.Name, err)
	}
	f, err := readFacts(ctx, c.workspaces, c.branches, agent, s.ID.Branch(), s.Workspace.Path, creds)
	if err != nil {
		return err
	}
	rs := reasons(f)
	// The person must have been told on their own request, with time to read, of the work as it
	// still is: unused since it was read for the notice; and only of these reasons. The time to read is counted on both clocks,
	// each against its own: on the chat's, as the person put the reactions; on the control plane's,
	// as they came, which may be together after a delay.
	n, told := c.toldOf(s.ID)
	forced := told && n.by == q.by && n.usedAt.Equal(s.LastUsedAt) &&
		n.chatAt.Add(readingTime).Before(q.at) && n.chatAt.Add(forceWindow).After(q.at) &&
		n.at.Add(readingTime).Before(q.received) &&
		!slices.ContainsFunc(rs, func(r reason) bool { return !slices.Contains(n.reasons, r.name()) })
	if force && !forced && len(rs) == 0 {
		// A :fire: not taken as forcing removes nothing: it may be praise.
		c.setTold(s.ID, nil)
		return c.tell(ctx, s, "このスレッドのセッションに、片付けを止めるものはありません。"+
			"片付けるなら、このスレッドの先頭（ルート）のメッセージに :"+CleanReaction+": を付けてください。")
	}
	onFire := force
	force = force && forced
	if keeps(rs, force) {
		// A :fire: taken as forcing, kept only by an open pull request, leaves the person able to force.
		return c.tellKept(ctx, s, rs, onFire && !force, q)
	}
	if c.holder != nil {
		released, err := c.holder.Release(ctx, s.ID)
		if err != nil {
			return fmt.Errorf("release session: %w", err)
		}
		if !released {
			return c.tell(ctx, s, "エージェントが作業しているあいだは、このスレッドのセッションを片付けません。"+
				"作業が終わってから、リアクションを外して付け直してください。")
		}
	}
	// Used since it was judged, it may hold work the judgement did not see. Unused is LastUsedAt as
	// read, not before it: a clock set back writes earlier times.
	branches, discarded, err := c.remove(ctx, agent, s.ID, creds, func(now domain.Session) bool { return now.LastUsedAt.Equal(s.LastUsedAt) }, force, n.reasons)
	var found workFound
	switch {
	case errors.Is(err, domain.ErrSessionNotFound):
		c.setTold(s.ID, nil)
		return nil
	case errors.As(err, &found):
		// The thread is told what kept it, so that it may be forced knowingly; a :fire: stopped by
		// what was not told, as before the lock, asks for :wastebasket: first.
		return c.tellKept(ctx, s, found.reasons, onFire, q)
	case errors.Is(err, domain.ErrSessionBusy), errors.Is(err, errKept):
		return c.tell(ctx, s, "このスレッドのセッションは、片付ける間に使われたので消していません。"+
			"エージェントの作業が終わってから、リアクションを外して付け直してください。")
	case err != nil:
		return err
	}
	c.setTold(s.ID, nil)
	return c.tell(ctx, s, removedNowText(branches, discarded))
}

func (c *SessionCleaner) tell(ctx context.Context, s domain.Session, text string) error {
	if err := c.notifier.Notify(ctx, s, text); err != nil {
		return fmt.Errorf("%w: %w", errNotTold, err)
	}
	return nil
}

const (
	// CleanReaction and ForceCleanReaction, on the message that started a session's thread, ask
	// for the session to be cleaned now (CleanNow), and to be despite work found nowhere else.
	CleanReaction      = "wastebasket"
	ForceCleanReaction = "fire"
)

// cleanHint tells how to have the session cleaned, ending a message about kept work.
const cleanHint = "エージェントに「締めて」と頼むと、残すものを Issue に残し、自分で解けるものを解いて、解けずに残るものを知らせます。" +
	"片付けを今すぐ回すには、このスレッドの先頭（ルート）のメッセージに :" + CleanReaction + ": を付けてください。"

// tellKept tells the thread the reasons that keep the session. Told on :wastebasket:, the person
// who put it may then force (told); told on a :fire: not taken as forcing, no one may until they
// put :wastebasket: themselves.
func (c *SessionCleaner) tellKept(ctx context.Context, s domain.Session, rs []reason, onFire bool, q cleanRequest) error {
	if err := c.tell(ctx, s, keptNowText(rs, !onFire)); err != nil {
		return err
	}
	if onFire {
		c.setTold(s.ID, nil)
		return nil
	}
	now := c.now()
	c.setTold(s.ID, &notice{by: q.by, at: now, chatAt: q.at.Add(now.Sub(q.received)), usedAt: s.LastUsedAt, reasons: names(rs)})
	return nil
}

func (c *SessionCleaner) toldOf(id domain.SessionID) (notice, bool) {
	c.toldMu.Lock()
	defer c.toldMu.Unlock()
	n, ok := c.told[id]
	return n, ok
}

// setTold records the notice of the session, or forgets it when nil, and forgets those past the
// time a :fire: may follow them.
func (c *SessionCleaner) setTold(id domain.SessionID, n *notice) {
	c.toldMu.Lock()
	defer c.toldMu.Unlock()
	delete(c.told, id)
	if n != nil {
		c.told[id] = *n
	}
	for other, n := range c.told {
		if n.at.Add(forceWindow).Before(c.now()) {
			delete(c.told, other)
		}
	}
}

func keptNowText(rs []reason, onClean bool) string {
	var b strings.Builder
	b.WriteString("このスレッドのセッションは、次の理由で消していません。\n")
	for _, r := range rs {
		b.WriteString("\n- ")
		b.WriteString(r.text())
	}
	b.WriteString("\n\n")
	if slices.ContainsFunc(rs, blocksForce) {
		b.WriteString("開いている PR があるあいだは、:" + ForceCleanReaction + ": でも消しません。PR をマージするか閉じてから、" +
			"リアクションを外して付け直してください。")
	} else if onClean {
		b.WriteString("失われるものを承知で消すなら、:" + CleanReaction + ": を付けた人が 1 時間のうちに、このスレッドの先頭（ルート）の" +
			"メッセージに :" + ForceCleanReaction + ": を付けてください（付けてあれば、外して付け直してください）。")
	} else {
		b.WriteString("失われるものを承知で消すなら、まず :" + ForceCleanReaction + ": を外し、このスレッドの先頭（ルート）のメッセージに :" +
			CleanReaction + ": を付けて（付けてあれば、外して付け直して）この理由を確かめてから、:" + ForceCleanReaction +
			": を付け直してください。:" + ForceCleanReaction + ": が効くのは、自分で付けた :" + CleanReaction +
			": に理由が返ってから 1 時間のうちに、そのあとエージェントが動いていない（依頼も、エージェントが自分で始めたターンも、バックグラウンドの処理の終わりもない）ときだけです。")
	}
	b.WriteString("\n\nエージェントに「締めて」と頼むと、残すものを Issue に残し、自分で解けるものを解いて、解けずに残るものを知らせます。" +
		"解けたら、:" + CleanReaction + ": を外して付け直してください。")
	return b.String()
}

func removedNowText(branches []string, discarded []reason) string {
	removed := "作業領域、会話の履歴"
	for _, b := range branches {
		removed += fmt.Sprintf("、ブランチ `%s`", b)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "依頼により、このスレッドのセッションを片付けました。\n\n消したもの：%s", removed)
	if len(discarded) > 0 {
		b.WriteString("\n\n承知のうえで捨てたもの：")
		for _, r := range discarded {
			b.WriteString("\n- ")
			b.WriteString(r.text())
		}
	}
	b.WriteString("\n\nこのスレッドで続けて依頼すると、前の会話を引き継がない新しいセッションとして始まります。")
	return b.String()
}
