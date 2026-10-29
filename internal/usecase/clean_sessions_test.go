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

// fakeWork holds the state of each session's workspace, by the session ID that names its path.
type fakeWork map[domain.SessionID]usecase.WorkspaceState

func workspacePath(id domain.SessionID) string { return "/sessions/" + string(id) }

func (f fakeWork) State(_ context.Context, _ domain.Agent, path string) (usecase.WorkspaceState, error) {
	return f[domain.SessionID(strings.TrimPrefix(path, "/sessions/"))], nil
}

type fakeBranches struct {
	sessions *fakeSessions
	// remote are the branches on GitHub by name; a missing one is gone and keeps nothing.
	remote  map[string]usecase.BranchFacts
	err     error
	removed []string
	// onRemove runs as Remove begins.
	onRemove func()
}

func (f *fakeBranches) List(_ context.Context, _ domain.Agent, creds domain.Credentials, prefix string) ([]string, error) {
	if creds.GitHubToken == "" {
		return nil, errors.New("no token")
	}
	var names []string
	for name, b := range f.remote {
		if b.Exists && strings.HasPrefix(name, prefix) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, f.err
}

func (f *fakeBranches) Inspect(_ context.Context, _ domain.Agent, _ domain.Credentials, queries []usecase.BranchQuery) ([]usecase.BranchFacts, error) {
	if f.err != nil {
		return nil, f.err
	}
	var facts []usecase.BranchFacts
	for _, q := range queries {
		b, ok := f.remote[q.Name]
		if !ok {
			b = usecase.BranchFacts{Name: q.Name}
		}
		facts = append(facts, b)
	}
	return facts, nil
}

func (f *fakeBranches) Remove(_ context.Context, _ domain.Agent, _ domain.Credentials, names []string) ([]string, error) {
	if f.onRemove != nil {
		f.onRemove()
	}
	if !f.sessions.anyLocked() {
		return nil, errors.New("removed without the lock")
	}
	f.removed = append(f.removed, names...)
	return names, nil
}

func (f *fakeSessions) anyLocked() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.locks {
		if n > 0 {
			return true
		}
	}
	return false
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
	// messages are the IDs of the posts, in the same order.
	messages map[domain.SessionID][]string
	sent     int
}

func (f *fakeNotifier) Notify(_ context.Context, s domain.Session, text string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if f.posts == nil {
		f.posts = make(map[domain.SessionID][]string)
		f.messages = make(map[domain.SessionID][]string)
	}
	f.sent++
	message := fmt.Sprintf("1700000000.%06d", f.sent)
	f.posts[s.ID] = append(f.posts[s.ID], text)
	f.messages[s.ID] = append(f.messages[s.ID], message)
	return message, nil
}

// lastMessage is the ID of the last post to the session's thread.
func (f *fakeNotifier) lastMessage(id domain.SessionID) string {
	ms := f.messages[id]
	if len(ms) == 0 {
		return ""
	}
	return ms[len(ms)-1]
}

type cleanEnv struct {
	sessions *fakeSessions
	work     fakeWork
	branches *fakeBranches
	files    *fakeFiles
	notifier *fakeNotifier
	now      time.Time
	cleaner  *usecase.SessionCleaner
}

const retention = 30 * 24 * time.Hour

func newCleanEnv() *cleanEnv {
	e := &cleanEnv{now: time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC), notifier: &fakeNotifier{}}
	e.sessions = &fakeSessions{saved: make(map[domain.SessionID]domain.Session)}
	e.work = make(fakeWork)
	e.branches = &fakeBranches{sessions: e.sessions, remote: make(map[string]usecase.BranchFacts)}
	e.files = &fakeFiles{sessions: e.sessions}
	e.cleaner = usecase.NewSessionCleaner(usecase.SessionCleanerDeps{
		Agents: fakeRegistry{dev.Name: dev}, Sessions: e.sessions, Credentials: &fakeIssuer{},
		Workspaces: e.work, Branches: e.branches, Files: e.files, Notifier: e.notifier,
	}, retention, func() time.Time { return e.now })
	return e
}

// cleanThread is the conversation the saved sessions started in.
var cleanThread = domain.Conversation{Platform: "slack", Workspace: "T1", Channel: "C1", Thread: "1700000000.000000"}

func (e *cleanEnv) save(id domain.SessionID, agent domain.AgentName, lastUsed time.Time) {
	e.sessions.saved[id] = domain.Session{ID: id, AgentName: agent, Conversation: cleanThread, Workspace: domain.Workspace{Path: workspacePath(id)}, LastUsedAt: lastUsed}
}

// merged is a branch on GitHub whose tip is in the default branch.
func merged(name string) usecase.BranchFacts {
	tip := "tip-" + name
	return usecase.BranchFacts{Name: name, Exists: true, Tip: tip,
		Commits: map[string]usecase.CommitPlaces{tip: {InDefault: true, InBranch: true}}}
}

// unmerged is a branch on GitHub whose tip is only in itself.
func unmerged(name string) usecase.BranchFacts {
	tip := "tip-" + name
	return usecase.BranchFacts{Name: name, Exists: true, Tip: tip,
		Commits: map[string]usecase.CommitPlaces{tip: {InBranch: true}}}
}

func TestSessionCleaner_Clean(t *testing.T) {
	const (
		fresh    domain.SessionID = "f0000000-0000-4000-8000-000000000001"
		done     domain.SessionID = "d0000000-0000-4000-8000-000000000002"
		review   domain.SessionID = "a0000000-0000-4000-8000-000000000003"
		inUse    domain.SessionID = "b0000000-0000-4000-8000-000000000004"
		orphan   domain.SessionID = "c0000000-0000-4000-8000-000000000005"
		gone     domain.SessionID = "e0000000-0000-4000-8000-000000000006"
		boundary domain.SessionID = "10000000-0000-4000-8000-000000000007"
		dirty    domain.SessionID = "20000000-0000-4000-8000-000000000008"
	)
	e := newCleanEnv()
	expired := e.now.Add(-retention - time.Second)
	for id, used := range map[domain.SessionID]time.Time{
		fresh: e.now.Add(-time.Hour), done: expired, review: expired, inUse: expired, boundary: e.now.Add(-retention), dirty: expired,
	} {
		e.save(id, dev.Name, used)
	}
	e.save(gone, "removed-agent", expired)
	e.sessions.orphans = map[domain.SessionID]bool{orphan: true}
	e.sessions.held = map[domain.SessionID]bool{inUse: true}
	// done created two branches, one only on GitHub, and pushed to a branch it did not create.
	e.branches.remote["acp/d0000000"] = merged("acp/d0000000")
	e.branches.remote["acp/d0000000-docs"] = merged("acp/d0000000-docs")
	e.branches.remote["shared"] = unmerged("shared")
	e.work[done] = usecase.WorkspaceState{Pushed: []usecase.BranchTip{{Name: "acp/d0000000", Tip: "tip-acp/d0000000"}, {Name: "shared", Tip: "tip-shared"}}}
	review7 := unmerged("acp/a0000000")
	review7.OpenHead = []usecase.PullRequest{{Number: 7, Title: "Fix", URL: "https://github.com/o/r/pull/7"}}
	e.branches.remote["acp/a0000000"] = review7
	e.work[dirty] = usecase.WorkspaceState{Changes: true}

	report, err := e.cleaner.Clean(context.Background())

	if !errors.Is(err, domain.ErrAgentNotFound) {
		t.Errorf("Clean() error = %v, want the session of the removed agent reported", err)
	}
	if want := []domain.SessionID{orphan, done}; !slices.Equal(sortedCopy(report.Removed), want) || !slices.Equal(sortedCopy(e.sessions.removed), want) {
		t.Errorf("Removed = %v, sessions removed = %v, want %v", report.Removed, e.sessions.removed, want)
	}
	if want := []domain.SessionID{dirty, review}; !slices.Equal(sortedCopy(report.Kept), want) {
		t.Errorf("Kept = %v, want %v", report.Kept, want)
	}
	if want := []string{"acp/d0000000", "acp/d0000000-docs"}; !slices.Equal(e.branches.removed, want) {
		t.Errorf("branches removed = %v, want only those the session created, %v", e.branches.removed, want)
	}
	if want := []domain.SessionID{done}; !slices.Equal(e.files.removed, want) {
		t.Errorf("files removed = %v, want %v", e.files.removed, want)
	}
	for id, want := range map[domain.SessionID][]string{
		done:   {"`acp/d0000000-docs`", "新しいセッション"},
		review: {"https://github.com/o/r/pull/7"},
		dirty:  {"コミットしていない変更"},
	} {
		p := e.notifier.posts[id]
		if len(p) != 1 || !strings.Contains(p[0], want[0]) || !strings.Contains(p[0], want[len(want)-1]) {
			t.Errorf("notices for %s = %q, want one mentioning %q", id, p, want)
		}
	}
	if len(e.notifier.posts) != 3 {
		t.Errorf("notified %v, want the removed and the kept sessions", e.notifier.posts)
	}
	for _, id := range []domain.SessionID{review, dirty} {
		if got := e.sessions.saved[id].KeptNotifiedAt; !got.Equal(e.now) {
			t.Errorf("KeptNotifiedAt of %s = %v, want %v", id, got, e.now)
		}
	}
	for id, n := range e.sessions.locks {
		if n != 0 {
			t.Errorf("session %s is still locked", id)
		}
	}
}

func TestSessionCleaner_NotifiesKeptOncePerRetention(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-2*retention))
	e.work[existingID] = usecase.WorkspaceState{Stash: true}
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

func TestSessionCleaner_KeepsSessionWhenBranchesFail(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-2*retention))
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

func TestSessionCleaner_KeepsSessionUsedMeanwhile(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-2*retention))
	e.sessions.onLock = func(id domain.SessionID) {
		e.sessions.onLock = nil
		s := e.sessions.saved[id]
		s.LastUsedAt = e.now
		e.sessions.saved[id] = s
	}
	report, err := e.cleaner.Clean(context.Background())
	if err != nil || len(report.Removed) != 0 || len(e.files.removed) != 0 {
		t.Fatalf("Clean() = %+v, %v, files removed %v; want the session kept", report, err, e.files.removed)
	}
}

func TestSessionCleaner_KeepsSessionWithWorkFoundUnderLock(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-2*retention))
	e.branches.remote["acp/0f8fad5b"] = merged("acp/0f8fad5b")
	e.sessions.onLock = func(domain.SessionID) {
		e.sessions.onLock = nil
		e.branches.remote["acp/0f8fad5b"] = unmerged("acp/0f8fad5b")
	}
	report, err := e.cleaner.Clean(context.Background())
	if err != nil || len(report.Removed) != 0 || len(e.branches.removed) != 0 || len(e.files.removed) != 0 {
		t.Fatalf("Clean() = %+v, %v, branches removed %v; want the session kept", report, err, e.branches.removed)
	}
}

func TestSessionCleaner_RemovesEvenIfNoticeFails(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-2*retention))
	e.notifier.err = errors.New("slack is down")
	report, err := e.cleaner.Clean(context.Background())
	if err == nil || !slices.Equal(report.Removed, []domain.SessionID{existingID}) {
		t.Fatalf("Clean() = %+v, %v; want the session removed and the failed notice reported", report, err)
	}
}
