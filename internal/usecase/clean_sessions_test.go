package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

type fakeBranches struct {
	inReview map[domain.SessionID]bool
	err      error
	removed  []domain.SessionID
}

func (f *fakeBranches) Remove(_ context.Context, _ domain.Agent, id domain.SessionID, creds domain.Credentials) error {
	if creds.GitHubToken == "" {
		return errors.New("no token")
	}
	if f.inReview[id] {
		return fmt.Errorf("%w: %s", usecase.ErrBranchInReview, id.Branch())
	}
	if f.err != nil {
		return f.err
	}
	f.removed = append(f.removed, id)
	return nil
}

type fakeFiles struct {
	sessions *fakeSessions
	removed  []domain.SessionID
}

func (f *fakeFiles) Remove(_ context.Context, _ domain.AgentName, id domain.SessionID) error {
	if _, err := f.sessions.Find(context.Background(), id); err != nil {
		return errors.New("files removed after the record")
	}
	f.removed = append(f.removed, id)
	return nil
}

func TestSessionCleaner_Clean(t *testing.T) {
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	const (
		fresh    domain.SessionID = "10000000-0000-4000-8000-000000000001"
		old      domain.SessionID = "10000000-0000-4000-8000-000000000002"
		review   domain.SessionID = "10000000-0000-4000-8000-000000000003"
		inUse    domain.SessionID = "10000000-0000-4000-8000-000000000004"
		orphan   domain.SessionID = "10000000-0000-4000-8000-000000000005"
		gone     domain.SessionID = "10000000-0000-4000-8000-000000000006"
		boundary domain.SessionID = "10000000-0000-4000-8000-000000000007"
	)
	retention := 30 * 24 * time.Hour
	expired := now.Add(-retention - time.Second)
	sessions := &fakeSessions{
		saved: map[domain.SessionID]domain.Session{
			fresh:    {ID: fresh, AgentName: dev.Name},
			old:      {ID: old, AgentName: dev.Name},
			review:   {ID: review, AgentName: dev.Name},
			inUse:    {ID: inUse, AgentName: dev.Name},
			gone:     {ID: gone, AgentName: "removed-agent"},
			boundary: {ID: boundary, AgentName: dev.Name},
		},
		orphans: map[domain.SessionID]bool{orphan: true},
		lastUsed: map[domain.SessionID]time.Time{
			fresh: now.Add(-time.Hour), old: expired, review: expired, inUse: expired, orphan: expired, gone: expired,
			boundary: now.Add(-retention),
		},
		held: map[domain.SessionID]bool{inUse: true},
	}
	branches := &fakeBranches{inReview: map[domain.SessionID]bool{review: true}}
	files := &fakeFiles{sessions: sessions}
	cleaner := usecase.NewSessionCleaner(fakeRegistry{dev.Name: dev}, sessions, &fakeIssuer{}, branches, files, retention, func() time.Time { return now })

	report, err := cleaner.Clean(context.Background())

	if !errors.Is(err, domain.ErrAgentNotFound) {
		t.Errorf("Clean() error = %v, want the session of the removed agent reported", err)
	}
	slices.Sort(report.Removed)
	if want := []domain.SessionID{old, orphan}; !slices.Equal(report.Removed, want) {
		t.Errorf("Removed = %v, want %v", report.Removed, want)
	}
	if want := []domain.SessionID{review}; !slices.Equal(report.InReview, want) {
		t.Errorf("InReview = %v, want %v", report.InReview, want)
	}
	slices.Sort(sessions.removed)
	if want := []domain.SessionID{old, orphan}; !slices.Equal(sessions.removed, want) {
		t.Errorf("sessions removed = %v, want %v", sessions.removed, want)
	}
	if want := []domain.SessionID{old}; !slices.Equal(branches.removed, want) || !slices.Equal(files.removed, want) {
		t.Errorf("branches removed = %v, files removed = %v, want %v", branches.removed, files.removed, want)
	}
	for id, n := range sessions.locks {
		if n != 0 {
			t.Errorf("session %s is still locked", id)
		}
	}
}

func TestSessionCleaner_KeepsSessionWhenBranchesFail(t *testing.T) {
	now := time.Now()
	sessions := &fakeSessions{
		saved:    map[domain.SessionID]domain.Session{existingID: {ID: existingID, AgentName: dev.Name}},
		lastUsed: map[domain.SessionID]time.Time{existingID: now.Add(-48 * time.Hour)},
	}
	files := &fakeFiles{sessions: sessions}
	cleaner := usecase.NewSessionCleaner(fakeRegistry{dev.Name: dev}, sessions, &fakeIssuer{},
		&fakeBranches{err: errors.New("github is down")}, files, time.Hour, func() time.Time { return now })

	report, err := cleaner.Clean(context.Background())
	if err == nil || len(report.Removed) != 0 {
		t.Fatalf("Clean() = %+v, %v, want an error and nothing removed", report, err)
	}
	if _, err := sessions.Find(context.Background(), existingID); err != nil || len(files.removed) != 0 {
		t.Errorf("session was partly removed: find error = %v, files removed = %v", err, files.removed)
	}
}

func TestDispatcher_SessionRemovedBeforeLock(t *testing.T) {
	e := newEnv(t, usecase.DispatcherOptions{})
	e.sessions.onLock = func(id domain.SessionID) {
		e.sessions.onLock = nil
		e.sessions.mu.Lock()
		defer e.sessions.mu.Unlock()
		delete(e.sessions.saved, id)
	}
	done := e.execute(usecase.RunAgentInput{AgentName: dev.Name, SessionID: existingID, CreateIfMissing: true, Prompt: "hi"})

	p := e.runner.next(t)
	eventually(t, func() bool { return len(p.sent()) == 1 })
	p.finish("done")
	o := wait(t, done)
	if o.err != nil || o.out.Session.ID != existingID || o.out.Session.Turns != 1 || e.workspaces.prepared != 1 {
		t.Fatalf("Execute() = %+v, %v, prepared %d; want a new session with the same ID", o.out, o.err, e.workspaces.prepared)
	}
}
