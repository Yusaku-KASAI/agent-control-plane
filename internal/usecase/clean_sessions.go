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
	holder      SessionHolder
	retention   time.Duration
	// cleaning runs the cleanings of one session one at a time, so that a later one finds what
	// an earlier one did: removed, or kept and told.
	cleaningMu sync.Mutex
	cleaning   map[domain.SessionID]*cleaningLock
	now        func() time.Time
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
		cleaning: make(map[domain.SessionID]*cleaningLock),
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
	branches, err := c.remove(ctx, agent, id, creds, cutoff, false, nil)
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
func (c *SessionCleaner) remove(ctx context.Context, agent domain.Agent, id domain.SessionID, creds domain.Credentials, cutoff time.Time, force bool, told []string) ([]string, error) {
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
	// Forced, it lets go only of what the thread was told.
	if rs := reasons(f); keeps(rs, force) || (force && slices.ContainsFunc(rs, func(r reason) bool { return !slices.Contains(told, r.name()) })) {
		return nil, workFound{rs}
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
	return c.update(ctx, s.ID, func(s *domain.Session) { s.KeptNotifiedAt, s.KeptReasons, s.KeptAskedBy = c.now(), names(rs), nil })
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
// covers the control plane's clock running behind Slack's, whose time the reactions carry.
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

// CleanNow judges one session now, whatever its retention, as a person asked in its thread, and
// tells the thread what came of it: removed, or kept and why. An agent waiting idle is let go of
// first; one at work keeps the session. With force, the work found nowhere else does not keep it
// (keeps), once the thread was told of it before askedAt, when the person asked, and since the
// session was last used: forcing before only tells. What the judgement under the lock finds
// beyond what was told keeps the session too.
func (c *SessionCleaner) CleanNow(ctx context.Context, id domain.SessionID, force bool, by domain.Requester, askedAt time.Time) error {
	defer c.cleanOne(id)()
	s, err := c.sessions.Find(ctx, id)
	if errors.Is(err, domain.ErrSessionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if c.holder != nil {
		released, err := c.holder.Release(ctx, id)
		if err != nil {
			return err
		}
		if !released {
			return c.notifier.Notify(ctx, s, "エージェントが作業しているあいだは、このスレッドのセッションを片付けません。"+
				"作業が終わってから、リアクションを外して付け直してください。")
		}
	}
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
	// The person must have been told before asking, with time to read, and since the agent last
	// changed the work; and only of these reasons (told).
	forced := s.KeptAskedBy != nil && *s.KeptAskedBy == by &&
		s.KeptNotifiedAt.After(s.LastUsedAt) && s.KeptNotifiedAt.Add(readingTime).Before(askedAt) &&
		s.KeptNotifiedAt.Add(forceWindow).After(askedAt) &&
		!slices.ContainsFunc(rs, func(r reason) bool { return !slices.Contains(s.KeptReasons, r.name()) })
	if force && !forced && len(rs) == 0 {
		// A :fire: not taken as forcing removes nothing: it may be praise.
		return c.notifier.Notify(ctx, s, "このスレッドのセッションに、片付けを止めるものはありません。"+
			"片付けるなら、このスレッドの最初のメッセージに :"+CleanReaction+": を付けてください。")
	}
	onFire := force
	force = force && forced
	if keeps(rs, force) {
		// A :fire: taken as forcing, kept only by an open pull request, leaves the person able to force.
		return c.tellKept(ctx, s, rs, onFire && !force, by)
	}
	// Used since it was judged, it may hold work the judgement did not see.
	branches, err := c.remove(ctx, agent, id, creds, s.LastUsedAt.Add(time.Nanosecond), force, s.KeptReasons)
	var found workFound
	switch {
	case errors.Is(err, domain.ErrSessionNotFound):
		return nil
	case errors.As(err, &found):
		// The thread is told what kept it, so that it may be forced knowingly.
		return c.tellKept(ctx, s, found.reasons, onFire && !force, by)
	case errors.Is(err, domain.ErrSessionBusy), errors.Is(err, errKept):
		return c.notifier.Notify(ctx, s, "このスレッドのセッションは、片付ける間に使われたので消していません。"+
			"エージェントの作業が終わってから、リアクションを外して付け直してください。")
	case err != nil:
		return err
	}
	return c.notifier.Notify(ctx, s, removedNowText(branches, rs))
}

const (
	// CleanReaction and ForceCleanReaction, on the message that started a session's thread, ask
	// for the session to be cleaned now (CleanNow), and to be despite work found nowhere else.
	CleanReaction      = "wastebasket"
	ForceCleanReaction = "fire"
)

// cleanHint tells how to have the session cleaned, ending a message about kept work.
const cleanHint = "エージェントに「締めて」と頼むと、残すものを Issue に残し、片付けられる状態にします。" +
	"片付けを今すぐ回すには、このスレッドの最初のメッセージに :" + CleanReaction + ": を付けてください。"

// tellKept tells the thread the reasons that keep the session and records them. Told on
// :wastebasket:, the person who put it may then force; told on a :fire: not taken as forcing, no
// one may until they put :wastebasket: themselves.
func (c *SessionCleaner) tellKept(ctx context.Context, s domain.Session, rs []reason, onFire bool, by domain.Requester) error {
	if err := c.notifier.Notify(ctx, s, keptNowText(rs, !onFire)); err != nil {
		return err
	}
	var asked *domain.Requester
	if !onFire {
		asked = &by
	}
	return c.update(ctx, s.ID, func(s *domain.Session) { s.KeptNotifiedAt, s.KeptReasons, s.KeptAskedBy = c.now(), names(rs), asked })
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
		b.WriteString("失われるものを承知で消すなら、:" + CleanReaction + ": を付けた人が 1 時間のうちに、このスレッドの最初の" +
			"メッセージに :" + ForceCleanReaction + ": を付けてください（付けてあれば、外して付け直してください）。")
	} else {
		b.WriteString("失われるものを承知で消すなら、まず :" + ForceCleanReaction + ": を外し、このスレッドの最初のメッセージに :" +
			CleanReaction + ": を付けて（付けてあれば、外して付け直して）この理由を確かめてから、:" + ForceCleanReaction +
			": を付け直してください。:" + ForceCleanReaction + ": が効くのは、自分で付けた :" + CleanReaction +
			": に理由が返ってから 1 時間のうちに、そのあと依頼せずに付けたときだけです。")
	}
	b.WriteString("\n\nエージェントに「締めて」と頼むと、残すものを Issue に残し、片付けられる状態にします。" +
		"片付いたら、:" + CleanReaction + ": を外して付け直してください。")
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
