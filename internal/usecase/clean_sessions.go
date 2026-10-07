package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// SessionCleaner removes sessions unused for longer than the retention period and tells their threads.
type SessionCleaner struct {
	agents      AgentRegistry
	sessions    SessionRepository
	credentials CredentialIssuer
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
	Branches    SessionBranches
	Files       SessionFiles
	Notifier    SessionNotifier
}

func NewSessionCleaner(deps SessionCleanerDeps, retention time.Duration, now func() time.Time) *SessionCleaner {
	return &SessionCleaner{
		agents: deps.Agents, sessions: deps.Sessions, credentials: deps.Credentials, branches: deps.Branches,
		files: deps.Files, notifier: deps.Notifier, retention: retention, now: now,
	}
}

type CleanReport struct {
	Removed []domain.SessionID
	// InReview lists expired sessions kept because a pull request from their branch is open.
	InReview []domain.SessionID
}

// errKept reports a session left as it is for now, such as one in use.
var errKept = errors.New("session kept")

// Clean removes every expired session that is not in use. It goes on past a session it cannot
// remove, which stays whole enough to be resumed or retried by a later run.
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
		case errors.Is(err, ErrBranchInReview):
			report.InReview = append(report.InReview, id)
		default:
			errs = errors.Join(errs, fmt.Errorf("session %s: %w", id, err))
		}
	}
	return report, errs
}

// clean checks for open pull requests before taking the lock, so that a session kept for review
// is not locked against requests while GitHub answers.
func (c *SessionCleaner) clean(ctx context.Context, id domain.SessionID) (removed bool, err error) {
	s, err := c.sessions.Find(ctx, id)
	if errors.Is(err, domain.ErrSessionNotFound) {
		return c.removeUnsaved(ctx, id)
	}
	if err != nil {
		return false, err
	}
	if s.LastUsedAt.IsZero() {
		// A record from before LastUsedAt existed counts as used now.
		return false, c.update(ctx, id, func(s *domain.Session) error {
			s.LastUsedAt = c.now()
			return nil
		})
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
	pulls, err := c.branches.OpenPullRequests(ctx, agent, id, creds)
	if err != nil {
		return false, fmt.Errorf("check pull requests: %w", err)
	}
	if len(pulls) > 0 {
		if s.ReviewNotifiedAt.Before(cutoff) {
			if err := c.notifyInReview(ctx, id, pulls); err != nil {
				return false, err
			}
		}
		return false, ErrBranchInReview
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

// remove deletes the record last: while the record exists the session ID still maps to this
// session, so a new session with the same ID cannot start beside remains of the old one.
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
	branches, err := c.branches.Remove(ctx, agent, id, creds)
	if err != nil {
		return nil, fmt.Errorf("remove branches: %w", err)
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

// notifyInReview records the notice only once it is posted, so that a failed one is retried.
func (c *SessionCleaner) notifyInReview(ctx context.Context, id domain.SessionID, pulls []PullRequest) error {
	return c.update(ctx, id, func(s *domain.Session) error {
		if err := c.notifier.Notify(ctx, *s, c.inReviewText(*s, pulls)); err != nil {
			return fmt.Errorf("notify pull requests: %w", err)
		}
		s.ReviewNotifiedAt = c.now()
		return nil
	})
}

// update changes the record under the lock and saves it unless change fails.
func (c *SessionCleaner) update(ctx context.Context, id domain.SessionID, change func(s *domain.Session) error) error {
	unlock, err := c.sessions.Lock(ctx, id)
	if err != nil {
		return err
	}
	defer unlock()
	s, err := c.sessions.Find(ctx, id)
	if err != nil {
		return err
	}
	if err := change(&s); err != nil {
		return err
	}
	return c.sessions.Save(ctx, s)
}

func (c *SessionCleaner) removedText(s domain.Session, branches []string) string {
	removed := "作業領域（push していない変更を含む）、会話の履歴"
	for _, b := range branches {
		removed += fmt.Sprintf("、ブランチ `%s`", b)
	}
	return fmt.Sprintf("このスレッドのセッションは%sのあいだ使われなかったため片付けました（最後に使ったのは %s）。\n\n"+
		"消したもの：%s\n\nこのスレッドで続けて依頼すると、前の会話を引き継がない新しいセッションとして始まります。",
		c.period(), s.LastUsedAt.Format("2006-01-02 15:04 MST"), removed)
}

func (c *SessionCleaner) inReviewText(s domain.Session, pulls []PullRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "このスレッドのセッションは%sのあいだ使われていませんが、次の PR が開いているため残しています（最後に使ったのは %s）。\n\n",
		c.period(), s.LastUsedAt.Format("2006-01-02 15:04 MST"))
	for _, p := range pulls {
		fmt.Fprintf(&b, "- [#%d %s](%s)\n", p.Number, p.Title, p.URL)
	}
	b.WriteString("\nマージするか閉じると、次の片付けでセッションを消します。続ける場合は、このスレッドで依頼してください。")
	return b.String()
}

func (c *SessionCleaner) period() string {
	const day = 24 * time.Hour
	if c.retention%day == 0 {
		return fmt.Sprintf(" %d 日", c.retention/day)
	}
	return " " + c.retention.String()
}
