package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// SessionCleaner removes sessions unused for longer than the retention period.
type SessionCleaner struct {
	agents      AgentRegistry
	sessions    SessionRepository
	credentials CredentialIssuer
	branches    SessionBranches
	files       SessionFiles
	retention   time.Duration
	now         func() time.Time
}

func NewSessionCleaner(
	agents AgentRegistry,
	sessions SessionRepository,
	credentials CredentialIssuer,
	branches SessionBranches,
	files SessionFiles,
	retention time.Duration,
	now func() time.Time,
) *SessionCleaner {
	return &SessionCleaner{
		agents: agents, sessions: sessions, credentials: credentials, branches: branches, files: files,
		retention: retention, now: now,
	}
}

type CleanReport struct {
	Removed []domain.SessionID
	// InReview lists expired sessions kept because a pull request from their branch is open.
	InReview []domain.SessionID
}

// Clean removes every expired session that is not in use. It goes on past a session it cannot
// remove, which stays whole enough to be resumed or retried by a later run.
func (c *SessionCleaner) Clean(ctx context.Context) (CleanReport, error) {
	stored, err := c.sessions.List(ctx)
	if err != nil {
		return CleanReport{}, fmt.Errorf("list sessions: %w", err)
	}
	cutoff := c.now().Add(-c.retention)
	var report CleanReport
	var errs error
	for _, s := range stored {
		if ctx.Err() != nil {
			return report, errors.Join(errs, ctx.Err())
		}
		if !s.LastUsed.Before(cutoff) {
			continue
		}
		err := c.remove(ctx, s.ID)
		switch {
		case err == nil:
			report.Removed = append(report.Removed, s.ID)
		case errors.Is(err, ErrBranchInReview):
			report.InReview = append(report.InReview, s.ID)
		case errors.Is(err, domain.ErrSessionBusy):
		default:
			errs = errors.Join(errs, fmt.Errorf("session %s: %w", s.ID, err))
		}
	}
	return report, errs
}

// remove checks for open pull requests before taking the lock, so that a session kept for review
// is not locked against requests while GitHub answers. It deletes the record last: while the
// record exists the session ID still maps to this session, so a new session with the same ID
// cannot start beside remains of the old one.
func (c *SessionCleaner) remove(ctx context.Context, id domain.SessionID) error {
	s, err := c.sessions.Find(ctx, id)
	if errors.Is(err, domain.ErrSessionNotFound) {
		return c.removeUnsaved(ctx, id)
	}
	if err != nil {
		return err
	}
	agent, err := c.agents.FindByName(ctx, s.AgentName)
	if err != nil {
		return fmt.Errorf("find agent %q: %w", s.AgentName, err)
	}
	creds, err := c.credentials.Issue(ctx, agent)
	if err != nil {
		return fmt.Errorf("issue credentials for agent %q: %w", agent.Name, err)
	}
	inReview, err := c.branches.InReview(ctx, agent, id, creds)
	if err != nil {
		return fmt.Errorf("check branches: %w", err)
	}
	if inReview {
		return ErrBranchInReview
	}

	unlock, err := c.sessions.Lock(ctx, id)
	if err != nil {
		return err
	}
	defer unlock()
	if err := c.branches.Remove(ctx, agent, id, creds); err != nil {
		return fmt.Errorf("remove branches: %w", err)
	}
	if err := c.files.Remove(ctx, agent.Name, id); err != nil {
		return fmt.Errorf("remove agent files: %w", err)
	}
	if err := c.sessions.Remove(ctx, id); err != nil {
		return fmt.Errorf("remove session: %w", err)
	}
	return nil
}

// removeUnsaved removes a session whose creation stopped before the record was saved, so that
// nothing else of it exists.
func (c *SessionCleaner) removeUnsaved(ctx context.Context, id domain.SessionID) error {
	unlock, err := c.sessions.Lock(ctx, id)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := c.sessions.Find(ctx, id); !errors.Is(err, domain.ErrSessionNotFound) {
		return fmt.Errorf("%w: being created", domain.ErrSessionBusy)
	}
	return c.sessions.Remove(ctx, id)
}
