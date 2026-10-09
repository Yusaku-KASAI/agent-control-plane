package usecase_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// fakeWork holds the branches of each session's workspace, by the session ID that names its path.
type fakeWork map[domain.SessionID]usecase.WorkspaceBranches

func workspacePath(id domain.SessionID) string { return "/sessions/" + string(id) }

func (f fakeWork) Branches(_ context.Context, path string) (usecase.WorkspaceBranches, error) {
	return f[domain.SessionID(strings.TrimPrefix(path, "/sessions/"))], nil
}

type fakeBranches struct {
	sessions *fakeSessions
	// states are the remote branches by name; a missing one is gone and contained in the default branch.
	states  map[string]usecase.RemoteBranch
	err     error
	removed []string
}

func (f *fakeBranches) Inspect(_ context.Context, _ domain.Agent, creds domain.Credentials, refs []usecase.BranchRef) ([]usecase.RemoteBranch, error) {
	if creds.GitHubToken == "" {
		return nil, errors.New("no token")
	}
	if f.err != nil {
		return nil, f.err
	}
	var branches []usecase.RemoteBranch
	for _, ref := range refs {
		b, ok := f.states[ref.Name]
		if !ok {
			b = usecase.RemoteBranch{State: usecase.BranchDone}
		}
		b.BranchRef = ref
		branches = append(branches, b)
	}
	return branches, nil
}

func (f *fakeBranches) Remove(_ context.Context, _ domain.Agent, _ domain.Credentials, names []string) ([]string, error) {
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
	e.branches = &fakeBranches{sessions: e.sessions}
	e.files = &fakeFiles{sessions: e.sessions}
	e.cleaner = usecase.NewSessionCleaner(usecase.SessionCleanerDeps{
		Agents: fakeRegistry{dev.Name: dev}, Sessions: e.sessions, Credentials: &fakeIssuer{},
		Workspaces: e.work, Branches: e.branches, Files: e.files, Notifier: e.notifier,
	}, retention, func() time.Time { return e.now })
	return e
}

func (e *cleanEnv) save(id domain.SessionID, agent domain.AgentName, lastUsed time.Time) {
	e.sessions.saved[id] = domain.Session{ID: id, AgentName: agent, Workspace: domain.Workspace{Path: workspacePath(id)}, LastUsedAt: lastUsed}
}

func TestSessionCleaner_Clean(t *testing.T) {
	const (
		fresh       domain.SessionID = "f0000000-0000-4000-8000-000000000001"
		done        domain.SessionID = "d0000000-0000-4000-8000-000000000002"
		review      domain.SessionID = "a0000000-0000-4000-8000-000000000003"
		inUse       domain.SessionID = "b0000000-0000-4000-8000-000000000004"
		orphan      domain.SessionID = "c0000000-0000-4000-8000-000000000005"
		gone        domain.SessionID = "e0000000-0000-4000-8000-000000000006"
		boundary    domain.SessionID = "10000000-0000-4000-8000-000000000007"
		unfinished  domain.SessionID = "20000000-0000-4000-8000-000000000008"
		unpushed    domain.SessionID = "30000000-0000-4000-8000-000000000009"
		neverPushed domain.SessionID = "40000000-0000-4000-8000-00000000000a"
	)
	e := newCleanEnv()
	expired := e.now.Add(-retention - time.Second)
	for id, used := range map[domain.SessionID]time.Time{
		fresh: e.now.Add(-time.Hour), done: expired, review: expired, inUse: expired, boundary: e.now.Add(-retention),
		unfinished: expired, unpushed: expired, neverPushed: expired,
	} {
		e.save(id, dev.Name, used)
	}
	e.save(gone, "removed-agent", expired)
	e.sessions.orphans = map[domain.SessionID]bool{orphan: true}
	e.sessions.held = map[domain.SessionID]bool{inUse: true}
	pushed := func(names ...string) []usecase.BranchRef {
		var refs []usecase.BranchRef
		for _, n := range names {
			refs = append(refs, usecase.BranchRef{Name: n, LocalTip: "tip-" + n})
		}
		return refs
	}
	unpushedBranch := func(name string) usecase.LocalBranch {
		return usecase.LocalBranch{Name: name, Tip: "tip-" + name, Unpushed: true}
	}
	e.work[done] = usecase.WorkspaceBranches{
		// Pushed to another session's branch too, which is that session's to delete.
		Pushed: pushed("acp/d0000000", "acp/e0000000", "acp/d0000000-docs"),
		// Squash-merged, deleted on GitHub and pruned: its commits are on no remote-tracking branch.
		Local: []usecase.LocalBranch{unpushedBranch("acp/d0000000-fix")},
	}
	e.work[review] = usecase.WorkspaceBranches{Pushed: pushed("acp/a0000000")}
	e.work[unfinished] = usecase.WorkspaceBranches{Pushed: pushed("acp/20000000")}
	e.work[unpushed] = usecase.WorkspaceBranches{Local: []usecase.LocalBranch{unpushedBranch("scratch"), {Name: "HEAD", Unpushed: true}}}
	e.work[neverPushed] = usecase.WorkspaceBranches{Local: []usecase.LocalBranch{unpushedBranch("acp/40000000")}}
	e.branches.states = map[string]usecase.RemoteBranch{
		"acp/d0000000": {State: usecase.BranchDone, Exists: true},
		// Not the session's to delete, so its pushed work stays on the remote and does not keep it.
		"acp/e0000000": {State: usecase.BranchUnfinished, Exists: true},
		"acp/a0000000": {State: usecase.BranchInReview, Exists: true,
			Pulls: []usecase.PullRequest{{Number: 7, Title: "Fix", URL: "https://github.com/o/r/pull/7"}}},
		"acp/20000000": {State: usecase.BranchUnfinished, Exists: true},
		"acp/40000000": {State: usecase.BranchUnfinished},
		"scratch":      {State: usecase.BranchUnfinished},
	}

	report, err := e.cleaner.Clean(context.Background())

	if !errors.Is(err, domain.ErrAgentNotFound) {
		t.Errorf("Clean() error = %v, want the session of the removed agent reported", err)
	}
	if want := []domain.SessionID{orphan, done}; !slices.Equal(sortedCopy(report.Removed), want) || !slices.Equal(sortedCopy(e.sessions.removed), want) {
		t.Errorf("Removed = %v, sessions removed = %v, want %v", report.Removed, e.sessions.removed, want)
	}
	if want := []domain.SessionID{unfinished, unpushed, neverPushed, review}; !slices.Equal(sortedCopy(report.Kept), want) {
		t.Errorf("Kept = %v, want %v", report.Kept, want)
	}
	if want := []string{"acp/d0000000"}; !slices.Equal(e.branches.removed, want) {
		t.Errorf("branches removed = %v, want only the existing ones the session created, %v", e.branches.removed, want)
	}
	if want := []domain.SessionID{done}; !slices.Equal(e.files.removed, want) {
		t.Errorf("files removed = %v, want %v", e.files.removed, want)
	}
	if len(e.notifier.posts) != 5 {
		t.Errorf("notified %v, want the removed and the kept sessions", e.notifier.posts)
	}
	for id, want := range map[domain.SessionID][]string{
		done:        {"`acp/d0000000`", "新しいセッション"},
		review:      {"https://github.com/o/r/pull/7"},
		unfinished:  {"`acp/20000000`", "既定のブランチ"},
		unpushed:    {"push していないコミットがあるブランチ：`scratch`", "detached HEAD"},
		neverPushed: {"push していないコミットがあるブランチ：`acp/40000000`"},
	} {
		p := e.notifier.posts[id]
		if len(p) != 1 {
			t.Errorf("notices for %s = %q, want one", id, p)
			continue
		}
		for _, w := range want {
			if !strings.Contains(p[0], w) {
				t.Errorf("notice for %s = %q, want it to mention %q", id, p[0], w)
			}
		}
		if id == neverPushed && strings.Contains(p[0], "リモートから消えた") {
			t.Errorf("notice for %s = %q, tells a branch never pushed as gone", id, p[0])
		}
	}
	for _, id := range []domain.SessionID{review, unfinished, unpushed, neverPushed} {
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
	e.work[existingID] = usecase.WorkspaceBranches{Local: []usecase.LocalBranch{{Name: "acp/0f8fad5b", Tip: "t", Unpushed: true}}}
	e.branches.states = map[string]usecase.RemoteBranch{"acp/0f8fad5b": {State: usecase.BranchUnfinished}}
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
	e.work[existingID] = usecase.WorkspaceBranches{Pushed: []usecase.BranchRef{{Name: "acp/0f8fad5b", LocalTip: "t"}}}
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
	e.work[existingID] = usecase.WorkspaceBranches{Pushed: []usecase.BranchRef{{Name: "acp/0f8fad5b", LocalTip: "t"}}}
	e.sessions.onLock = func(domain.SessionID) {
		e.sessions.onLock = nil
		e.branches.states = map[string]usecase.RemoteBranch{"acp/0f8fad5b": {Name: "acp/0f8fad5b", State: usecase.BranchUnfinished, Exists: true}}
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
