package usecase_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// A person asks in the thread: the session is judged now, however recently it was used.
func TestSessionCleaner_CleanNow(t *testing.T) {
	recently := func(e *cleanEnv) { e.save(existingID, dev.Name, e.now.Add(-1)) }
	told := func(e *cleanEnv) {
		s := e.sessions.saved[existingID]
		s.KeptNotifiedAt = e.now
		e.sessions.saved[existingID] = s
	}
	pull := usecase.PullRequest{Number: 7, Title: "fix", URL: "https://example.com/7"}
	tests := map[string]struct {
		arrange     func(e *cleanEnv)
		force       bool
		wantRemoved bool
		wantText    string
	}{
		"nothing keeps it": {
			arrange:     func(e *cleanEnv) { e.branches.remote["acp/0f8fad5b"] = merged("acp/0f8fad5b") },
			wantRemoved: true, wantText: "片付けました",
		},
		"work keeps it": {
			arrange:  func(e *cleanEnv) { e.branches.remote["acp/0f8fad5b"] = unmerged("acp/0f8fad5b") },
			wantText: ":fire:",
		},
		"forced once told, the work is let go": {
			arrange: func(e *cleanEnv) {
				e.work[existingID] = usecase.WorkspaceState{Changes: true}
				told(e)
			},
			force:       true,
			wantRemoved: true, wantText: "承知のうえで捨てたもの",
		},
		"forced after being used since told, it only tells again": {
			arrange: func(e *cleanEnv) {
				e.work[existingID] = usecase.WorkspaceState{Changes: true}
				s := e.sessions.saved[existingID]
				s.KeptNotifiedAt = e.now.Add(-40 * 24 * time.Hour)
				e.sessions.saved[existingID] = s
			},
			force:    true,
			wantText: ":fire:",
		},
		"forced before the notice, as in a burst of reactions, it only tells": {
			arrange: func(e *cleanEnv) {
				e.work[existingID] = usecase.WorkspaceState{Changes: true}
				s := e.sessions.saved[existingID]
				s.KeptNotifiedAt = e.now.Add(time.Hour)
				e.sessions.saved[existingID] = s
			},
			force:    true,
			wantText: ":fire:",
		},
		"forced before being told, it only tells": {
			arrange:  func(e *cleanEnv) { e.work[existingID] = usecase.WorkspaceState{Changes: true} },
			force:    true,
			wantText: ":fire:",
		},
		"forced, a pull request into its branch still keeps it": {
			arrange: func(e *cleanEnv) {
				b := merged("acp/0f8fad5b")
				b.OpenBase = []usecase.PullRequest{pull}
				e.branches.remote["acp/0f8fad5b"] = b
				told(e)
			},
			force:    true,
			wantText: "開いている PR があるあいだは",
		},
		"forced, an open pull request still keeps it": {
			arrange: func(e *cleanEnv) {
				b := unmerged("acp/0f8fad5b")
				b.OpenHead = []usecase.PullRequest{pull}
				e.branches.remote["acp/0f8fad5b"] = b
				told(e)
			},
			force:    true,
			wantText: "開いている PR があるあいだは",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			e := newCleanEnv()
			recently(e)
			tt.arrange(e)

			if err := e.cleaner.CleanNow(context.Background(), existingID, tt.force, e.now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}

			if removed := len(e.files.removed) > 0; removed != tt.wantRemoved {
				t.Errorf("removed = %v, want %v", removed, tt.wantRemoved)
			}
			posts := e.notifier.posts[existingID]
			if len(posts) != 1 || !strings.Contains(posts[0], tt.wantText) {
				t.Errorf("told %q, want one message with %q", posts, tt.wantText)
			}
		})
	}
}

// Used while being judged, it is kept and the thread is told so.
func TestSessionCleaner_CleanNowKeepsSessionUsedMeanwhile(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	e.sessions.onLock = func(id domain.SessionID) {
		e.sessions.onLock = nil
		s := e.sessions.saved[id]
		// Used after the judgement, though before the clock the cleaner reads.
		s.LastUsedAt = e.now.Add(-1).Add(time.Nanosecond)
		e.sessions.saved[id] = s
	}
	if err := e.cleaner.CleanNow(context.Background(), existingID, false, e.now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if posts := e.notifier.posts[existingID]; len(e.files.removed) != 0 || len(posts) != 1 || !strings.Contains(posts[0], "使われた") {
		t.Errorf("removed %v, told %q; want it kept and the thread told", e.files.removed, posts)
	}
}

// A thread with no session, such as one already cleaned, is left alone.
func TestSessionCleaner_CleanNowWithoutSession(t *testing.T) {
	e := newCleanEnv()
	if err := e.cleaner.CleanNow(context.Background(), existingID, false, e.now.Add(time.Second)); err != nil || len(e.notifier.posts) != 0 {
		t.Errorf("CleanNow() = %v, told %v; want nothing", err, e.notifier.posts)
	}
}

type fakeHolder struct {
	idle     bool
	released []domain.SessionID
}

func (f *fakeHolder) Release(_ context.Context, id domain.SessionID) (bool, error) {
	if f.idle {
		f.released = append(f.released, id)
	}
	return f.idle, nil
}

// An agent waiting idle is let go of so that the session is cleaned at once; one at work keeps it.
func TestSessionCleaner_CleanNowLetsGoOfIdleAgents(t *testing.T) {
	for _, idle := range []bool{true, false} {
		e := newCleanEnv()
		holder := &fakeHolder{idle: idle}
		cleaner := usecase.NewSessionCleaner(usecase.SessionCleanerDeps{
			Agents: fakeRegistry{dev.Name: dev}, Sessions: e.sessions, Credentials: &fakeIssuer{},
			Workspaces: e.work, Branches: e.branches, Files: e.files, Notifier: e.notifier, Holder: holder,
		}, retention, func() time.Time { return e.now })
		e.save(existingID, dev.Name, e.now.Add(-1))

		if err := cleaner.CleanNow(context.Background(), existingID, false, e.now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}

		posts := e.notifier.posts[existingID]
		if removed := len(e.files.removed) > 0; removed != idle || len(posts) != 1 {
			t.Errorf("idle %v: removed %v, told %q; want removed only when idle", idle, removed, posts)
		}
		if !idle && !strings.Contains(posts[0], "作業しているあいだ") {
			t.Errorf("told %q, want that the agent is at work", posts[0])
		}
	}
}

// Taken by another cleaning meanwhile, the session is gone: nothing contradicts that.
func TestSessionCleaner_CleanNowAfterAnotherCleaning(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	e.sessions.onLock = func(id domain.SessionID) {
		e.sessions.onLock = nil
		delete(e.sessions.saved, id)
	}
	if err := e.cleaner.CleanNow(context.Background(), existingID, false, e.now.Add(time.Second)); err != nil || len(e.notifier.posts) != 0 {
		t.Errorf("CleanNow() = %v, told %v; want nothing", err, e.notifier.posts)
	}
}

// Two cleanings of one session, as of two reactions in a row, run one after the other: the later
// finds the session gone and tells nothing.
func TestSessionCleaner_CleanNowTwiceAtOnce(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	e.branches.remote["acp/0f8fad5b"] = merged("acp/0f8fad5b")
	e.sessions.exclusive = true
	// The first holds the lock, removing, while the second comes.
	var once sync.Once
	e.branches.onRemove = func() { once.Do(func() { time.Sleep(100 * time.Millisecond) }) }
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			if err := e.cleaner.CleanNow(context.Background(), existingID, false, e.now.Add(time.Second)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if posts := e.notifier.posts[existingID]; len(posts) != 1 || !strings.Contains(posts[0], "片付けました") {
		t.Errorf("told %q, want only that it was cleaned", posts)
	}
}

// Forced, it lets go only of what the thread was told: what the judgement under the lock finds
// beyond that keeps the session.
func TestSessionCleaner_CleanNowForcedKeepsWhatWasNotTold(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	s := e.sessions.saved[existingID]
	s.KeptNotifiedAt = e.now
	e.sessions.saved[existingID] = s
	e.work[existingID] = usecase.WorkspaceState{Changes: true}
	e.sessions.onLock = func(domain.SessionID) {
		e.sessions.onLock = nil
		e.work[existingID] = usecase.WorkspaceState{Changes: true, Stash: true}
	}
	if err := e.cleaner.CleanNow(context.Background(), existingID, true, e.now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(e.files.removed) != 0 {
		t.Error("removed with a stash the thread was not told of")
	}
}
