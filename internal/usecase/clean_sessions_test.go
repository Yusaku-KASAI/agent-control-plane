package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

type fakeBranches struct {
	sessions *fakeSessions
	pulls    map[domain.SessionID][]usecase.PullRequest
	err      error
	removed  []domain.SessionID
}

func (f *fakeBranches) OpenPullRequests(_ context.Context, _ domain.Agent, id domain.SessionID, creds domain.Credentials) ([]usecase.PullRequest, error) {
	if creds.GitHubToken == "" {
		return nil, errors.New("no token")
	}
	if f.sessions.isLocked(id) {
		return nil, errors.New("checked under the lock")
	}
	return f.pulls[id], f.err
}

func (f *fakeBranches) Remove(_ context.Context, _ domain.Agent, id domain.SessionID, _ domain.Credentials) ([]string, error) {
	if !f.sessions.isLocked(id) {
		return nil, errors.New("removed without the lock")
	}
	if len(f.pulls[id]) > 0 {
		return nil, fmt.Errorf("%w: %s", usecase.ErrBranchInReview, id.Branch())
	}
	f.removed = append(f.removed, id)
	return []string{id.Branch()}, nil
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

type fakeNotifier struct {
	err   error
	posts map[domain.SessionID][]string
}

func (f *fakeNotifier) Notify(_ context.Context, s domain.Session, text string) error {
	if f.err != nil {
		return f.err
	}
	if f.posts == nil {
		f.posts = make(map[domain.SessionID][]string)
	}
	f.posts[s.ID] = append(f.posts[s.ID], text)
	return nil
}

type cleanEnv struct {
	sessions *fakeSessions
	branches *fakeBranches
	files    *fakeFiles
	notifier *fakeNotifier
	now      time.Time
	cleaner  *usecase.SessionCleaner
}

const retention = 30 * 24 * time.Hour

func newCleanEnv(saved ...domain.Session) *cleanEnv {
	e := &cleanEnv{now: time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC), notifier: &fakeNotifier{}}
	e.sessions = &fakeSessions{saved: make(map[domain.SessionID]domain.Session)}
	for _, s := range saved {
		e.sessions.saved[s.ID] = s
	}
	e.branches = &fakeBranches{sessions: e.sessions}
	e.files = &fakeFiles{sessions: e.sessions}
	e.cleaner = usecase.NewSessionCleaner(usecase.SessionCleanerDeps{
		Agents: fakeRegistry{dev.Name: dev}, Sessions: e.sessions, Credentials: &fakeIssuer{},
		Branches: e.branches, Files: e.files, Notifier: e.notifier,
	}, retention, func() time.Time { return e.now })
	return e
}

func TestSessionCleaner_Clean(t *testing.T) {
	const (
		fresh    domain.SessionID = "10000000-0000-4000-8000-000000000001"
		old      domain.SessionID = "10000000-0000-4000-8000-000000000002"
		review   domain.SessionID = "10000000-0000-4000-8000-000000000003"
		inUse    domain.SessionID = "10000000-0000-4000-8000-000000000004"
		orphan   domain.SessionID = "10000000-0000-4000-8000-000000000005"
		gone     domain.SessionID = "10000000-0000-4000-8000-000000000006"
		boundary domain.SessionID = "10000000-0000-4000-8000-000000000007"
	)
	e := newCleanEnv()
	expired := e.now.Add(-retention - time.Second)
	for id, used := range map[domain.SessionID]time.Time{
		fresh: e.now.Add(-time.Hour), old: expired, review: expired, inUse: expired, boundary: e.now.Add(-retention),
	} {
		e.sessions.saved[id] = domain.Session{ID: id, AgentName: dev.Name, LastUsedAt: used}
	}
	e.sessions.saved[gone] = domain.Session{ID: gone, AgentName: "removed-agent", LastUsedAt: expired}
	e.sessions.orphans = map[domain.SessionID]bool{orphan: true}
	e.sessions.held = map[domain.SessionID]bool{inUse: true}
	e.branches.pulls = map[domain.SessionID][]usecase.PullRequest{review: {{Number: 7, Title: "Fix", URL: "https://github.com/o/r/pull/7"}}}

	report, err := e.cleaner.Clean(context.Background())

	if !errors.Is(err, domain.ErrAgentNotFound) {
		t.Errorf("Clean() error = %v, want the session of the removed agent reported", err)
	}
	slices.Sort(report.Removed)
	if want := []domain.SessionID{old, orphan}; !slices.Equal(report.Removed, want) || !slices.Equal(sortedCopy(e.sessions.removed), want) {
		t.Errorf("Removed = %v, sessions removed = %v, want %v", report.Removed, e.sessions.removed, want)
	}
	if want := []domain.SessionID{review}; !slices.Equal(report.InReview, want) {
		t.Errorf("InReview = %v, want %v", report.InReview, want)
	}
	if want := []domain.SessionID{old}; !slices.Equal(e.branches.removed, want) || !slices.Equal(e.files.removed, want) {
		t.Errorf("branches removed = %v, files removed = %v, want %v", e.branches.removed, e.files.removed, want)
	}
	if len(e.notifier.posts) != 2 {
		t.Errorf("notified %v, want the removed and the kept session", e.notifier.posts)
	}
	if p := e.notifier.posts[old]; len(p) != 1 || !strings.Contains(p[0], "acp/10000000") || !strings.Contains(p[0], "新しいセッション") {
		t.Errorf("removal notice = %q", p)
	}
	if p := e.notifier.posts[review]; len(p) != 1 || !strings.Contains(p[0], "https://github.com/o/r/pull/7") {
		t.Errorf("review notice = %q", p)
	}
	if got := e.sessions.saved[review].ReviewNotifiedAt; !got.Equal(e.now) {
		t.Errorf("ReviewNotifiedAt = %v, want %v", got, e.now)
	}
	for id, n := range e.sessions.locks {
		if n != 0 {
			t.Errorf("session %s is still locked", id)
		}
	}
}

func TestSessionCleaner_NotifiesReviewOncePerRetention(t *testing.T) {
	e := newCleanEnv()
	e.sessions.saved[existingID] = domain.Session{ID: existingID, AgentName: dev.Name, LastUsedAt: e.now.Add(-2 * retention)}
	e.branches.pulls = map[domain.SessionID][]usecase.PullRequest{existingID: {{Number: 1}}}
	clean := func() {
		t.Helper()
		if _, err := e.cleaner.Clean(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	e.notifier.err = errors.New("slack is down")
	if _, err := e.cleaner.Clean(context.Background()); err == nil {
		t.Fatal("Clean() succeeded though the notice failed")
	}
	e.notifier.err = nil
	clean()
	e.now = e.now.Add(time.Hour)
	clean()
	if n := len(e.notifier.posts[existingID]); n != 1 {
		t.Fatalf("notices within one retention period = %d, want 1 after the failed one", n)
	}
	e.now = e.now.Add(retention)
	clean()
	if n := len(e.notifier.posts[existingID]); n != 2 {
		t.Errorf("notices after another retention period = %d, want 2", n)
	}
}

func TestSessionCleaner_RecordWithoutLastUse(t *testing.T) {
	e := newCleanEnv(domain.Session{ID: existingID, AgentName: dev.Name})
	report, err := e.cleaner.Clean(context.Background())
	if err != nil || len(report.Removed) != 0 {
		t.Fatalf("Clean() = %+v, %v; want nothing removed", report, err)
	}
	if got := e.sessions.saved[existingID].LastUsedAt; !got.Equal(e.now) {
		t.Errorf("LastUsedAt = %v, want the time it was first seen, %v", got, e.now)
	}
}

func TestSessionCleaner_KeepsSessionWhenBranchesFail(t *testing.T) {
	e := newCleanEnv()
	e.sessions.saved[existingID] = domain.Session{ID: existingID, AgentName: dev.Name, LastUsedAt: e.now.Add(-2 * retention)}
	e.branches.err = errors.New("github is down")

	report, err := e.cleaner.Clean(context.Background())
	if err == nil || len(report.Removed) != 0 {
		t.Fatalf("Clean() = %+v, %v, want an error and nothing removed", report, err)
	}
	if _, err := e.sessions.Find(context.Background(), existingID); err != nil || len(e.files.removed) != 0 || len(e.notifier.posts) != 0 {
		t.Errorf("session was partly removed: find error = %v, files removed = %v, posts = %v", err, e.files.removed, e.notifier.posts)
	}
}

func sortedCopy(ids []domain.SessionID) []domain.SessionID {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	return ids
}
