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
// threads. It keeps a session whose work is nowhere else yet.
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
	// Kept lists expired sessions kept for the work they hold.
	Kept []domain.SessionID
}

var (
	// errKept reports a session left as it is for now, such as one in use.
	errKept = errors.New("session kept")
	// errHasWork reports an expired session kept for the work it holds.
	errHasWork = errors.New("session holds work found nowhere else")
)

// work is what a session holds that may be found nowhere else.
type work struct {
	local  []LocalBranch
	remote []RemoteBranch
}

func (w work) remoteOf(b LocalBranch) (RemoteBranch, bool) {
	i := slices.IndexFunc(w.remote, func(r RemoteBranch) bool { return r.Name == b.Upstream && r.LocalTip == b.Tip })
	if i < 0 {
		return RemoteBranch{}, false
	}
	return w.remote[i], true
}

// unpushed lists the local branches holding commits found nowhere else. A branch gone from the
// remote is judged by GitHub from its local tip instead: once a merge has deleted the branch, a
// prune leaves its commits on no remote-tracking branch.
func (w work) unpushed() []string {
	var names []string
	for _, b := range w.local {
		if r, ok := w.remoteOf(b); b.Unpushed && (!ok || r.Exists || r.State != BranchDone) {
			names = append(names, b.Name)
		}
	}
	return names
}

func (w work) pending() bool {
	return len(w.unpushed()) > 0 || slices.ContainsFunc(w.remote, func(b RemoteBranch) bool { return b.State != BranchDone })
}

// removable lists the branches to delete: only those the session created, since the others may
// still be in use by whoever made them.
func (w work) removable(id domain.SessionID) []string {
	var names []string
	for _, b := range w.remote {
		if b.Exists && id.OwnsBranch(b.Name) && !slices.Contains(names, b.Name) {
			names = append(names, b.Name)
		}
	}
	return names
}

// Clean removes every expired session that is not in use and holds no pending work. It goes on
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

// clean looks for pending work before taking the lock, so that a kept session is not locked
// against requests while GitHub answers.
func (c *SessionCleaner) clean(ctx context.Context, id domain.SessionID) (removed bool, err error) {
	s, err := c.sessions.Find(ctx, id)
	if errors.Is(err, domain.ErrSessionNotFound) {
		return c.removeUnsaved(ctx, id)
	}
	if err != nil {
		return false, err
	}
	if s.LastUsedAt.IsZero() {
		// A record without LastUsedAt counts as used when first seen.
		err := c.update(ctx, id, func(s *domain.Session) { s.LastUsedAt = c.now() })
		if errors.Is(err, domain.ErrSessionNotFound) {
			return false, nil
		}
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
	w, err := c.work(ctx, agent, s, creds)
	if err != nil {
		return false, err
	}
	if w.pending() {
		if s.KeptNotifiedAt.Before(cutoff) {
			if err := c.notifyKept(ctx, s, w); err != nil {
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

func (c *SessionCleaner) work(ctx context.Context, agent domain.Agent, s domain.Session, creds domain.Credentials) (work, error) {
	local, err := c.workspaces.Branches(ctx, s.Workspace.Path)
	if err != nil {
		return work{}, fmt.Errorf("read workspace branches: %w", err)
	}
	var refs []BranchRef
	for _, b := range local {
		if ref := (BranchRef{Name: b.Upstream, LocalTip: b.Tip}); b.Upstream != "" && !slices.Contains(refs, ref) {
			refs = append(refs, ref)
		}
	}
	w := work{local: local}
	if len(refs) > 0 {
		if w.remote, err = c.branches.Inspect(ctx, agent, creds, refs); err != nil {
			return work{}, fmt.Errorf("inspect branches: %w", err)
		}
	}
	return w, nil
}

// remove deletes the record last: while the record exists the session ID still maps to this
// session, so a new session with the same ID cannot start beside remains of the old one. It looks
// for pending work again under the lock, as the agent may have pushed or a pull request may have
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
	w, err := c.work(ctx, agent, s, creds)
	if err != nil {
		return nil, err
	}
	if w.pending() {
		return nil, fmt.Errorf("%w: work found meanwhile", errKept)
	}
	var branches []string
	if names := w.removable(id); len(names) > 0 {
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
func (c *SessionCleaner) notifyKept(ctx context.Context, s domain.Session, w work) error {
	if err := c.notifier.Notify(ctx, s, c.keptText(s, w)); err != nil {
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
	removed := "作業領域（コミットしていない変更を含む）、会話の履歴"
	for _, b := range branches {
		removed += fmt.Sprintf("、ブランチ `%s`", b)
	}
	return fmt.Sprintf("このスレッドのセッションは、最後に使ってから%sたったため片付けました。\n\n"+
		"消したもの：%s\n\nこのスレッドで続けて依頼すると、前の会話を引き継がない新しいセッションとして始まります。",
		c.since(s), removed)
}

func (c *SessionCleaner) keptText(s domain.Session, w work) string {
	var b strings.Builder
	fmt.Fprintf(&b, "このスレッドのセッションは、最後に使ってから%sたちましたが、ほかにない作業が残っているため消さずに残しています。\n", c.since(s))
	var pulls []int
	for _, r := range w.remote {
		for _, p := range r.Pulls {
			if !slices.Contains(pulls, p.Number) {
				pulls = append(pulls, p.Number)
				fmt.Fprintf(&b, "\n- 開いている PR：#%d %s %s", p.Number, p.Title, p.URL)
			}
		}
	}
	for _, r := range w.remote {
		switch {
		case r.State != BranchUnfinished:
		case r.Exists:
			fmt.Fprintf(&b, "\n- 開いている PR がなく、既定のブランチにも入っていないブランチ：`%s`", r.Name)
		case !slices.ContainsFunc(w.local, func(l LocalBranch) bool { return l.Unpushed && l.Upstream == r.Name && l.Tip == r.LocalTip }):
			fmt.Fprintf(&b, "\n- リモートから消えたが、既定のブランチにも閉じた PR にも入っていないブランチ：`%s`", r.Name)
		}
	}
	for _, name := range w.unpushed() {
		if name == "HEAD" {
			b.WriteString("\n- どのブランチにもないコミット（detached HEAD）")
			continue
		}
		fmt.Fprintf(&b, "\n- push していないコミットがあるブランチ：`%s`", name)
	}
	b.WriteString("\n\nPR をマージするか閉じる、要らないブランチやコミットを消すなどで作業が片付くと、次の片付けでセッションを消します。" +
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
