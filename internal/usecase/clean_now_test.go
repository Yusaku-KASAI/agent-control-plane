package usecase_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// person puts the reactions in the tests.
var person = domain.Requester{Platform: "slack", ID: "U_ALICE"}

// A person asks in the thread: the session is judged now, however recently it was used.
func TestSessionCleaner_CleanNow(t *testing.T) {
	recently := func(e *cleanEnv) { e.save(existingID, dev.Name, e.now.Add(-1)) }
	// told has the thread told of the reasons, as by :wastebasket:, before the test's reaction.
	told := func(e *cleanEnv) {
		if err := e.cleaner.CleanNow(context.Background(), existingID, false, person, e.now); err != nil {
			panic(err)
		}
		e.notifier.posts = nil
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
				told(e)
				s := e.sessions.saved[existingID]
				s.LastUsedAt = s.KeptNotifiedAt.Add(time.Second)
				e.sessions.saved[existingID] = s
			},
			force:    true,
			wantText: ":fire:",
		},
		"forced with nothing keeping it, as in praise, it removes nothing": {
			arrange:  func(e *cleanEnv) { e.branches.remote["acp/0f8fad5b"] = merged("acp/0f8fad5b") },
			force:    true,
			wantText: ":wastebasket:",
		},
		"forced before the notice, as in a burst of reactions, it only tells": {
			arrange: func(e *cleanEnv) {
				e.work[existingID] = usecase.WorkspaceState{Changes: true}
				told(e)
				s := e.sessions.saved[existingID]
				s.KeptToldAt = e.now.Add(time.Hour)
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

			if err := e.cleaner.CleanNow(context.Background(), existingID, tt.force, person, e.now.Add(time.Minute)); err != nil {
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
	if err := e.cleaner.CleanNow(context.Background(), existingID, false, person, e.now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if posts := e.notifier.posts[existingID]; len(e.files.removed) != 0 || len(posts) != 1 || !strings.Contains(posts[0], "使われた") {
		t.Errorf("removed %v, told %q; want it kept and the thread told", e.files.removed, posts)
	}
}

// A thread with no session, such as one already cleaned, is left alone.
func TestSessionCleaner_CleanNowWithoutSession(t *testing.T) {
	e := newCleanEnv()
	if err := e.cleaner.CleanNow(context.Background(), existingID, false, person, e.now.Add(time.Minute)); err != nil || len(e.notifier.posts) != 0 {
		t.Errorf("CleanNow() = %v, told %v; want nothing", err, e.notifier.posts)
	}
}

type fakeHolder struct {
	idle  bool
	err   error
	asked []domain.SessionID
}

func (f *fakeHolder) Release(_ context.Context, id domain.SessionID) (bool, error) {
	f.asked = append(f.asked, id)
	return f.idle, f.err
}

func (e *cleanEnv) withHolder(h *fakeHolder) *usecase.SessionCleaner {
	return usecase.NewSessionCleaner(usecase.SessionCleanerDeps{
		Agents: fakeRegistry{dev.Name: dev}, Sessions: e.sessions, Credentials: &fakeIssuer{},
		Workspaces: e.work, Branches: e.branches, Files: e.files, Notifier: e.notifier, Holder: h,
	}, retention, func() time.Time { return e.now })
}

// An agent waiting idle is let go of so that the session is cleaned at once; one at work keeps it.
func TestSessionCleaner_CleanNowLetsGoOfIdleAgents(t *testing.T) {
	for _, idle := range []bool{true, false} {
		e := newCleanEnv()
		cleaner := e.withHolder(&fakeHolder{idle: idle})
		e.save(existingID, dev.Name, e.now.Add(-1))

		if err := cleaner.CleanNow(context.Background(), existingID, false, person, e.now.Add(time.Minute)); err != nil {
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

// A session that is not to be removed leaves its agent be: a :fire: in praise or a :wastebasket:
// on kept work does not stop the container the next request would use.
func TestSessionCleaner_CleanNowLetsGoOnlyToRemove(t *testing.T) {
	for name, tt := range map[string]struct {
		force bool
		work  usecase.WorkspaceState
	}{
		"kept for its work": {work: usecase.WorkspaceState{Changes: true}},
		"praised":           {force: true},
	} {
		t.Run(name, func(t *testing.T) {
			e := newCleanEnv()
			holder := &fakeHolder{idle: true}
			cleaner := e.withHolder(holder)
			e.save(existingID, dev.Name, e.now.Add(-1))
			e.work[existingID] = tt.work
			if err := cleaner.CleanNow(context.Background(), existingID, tt.force, person, e.now.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			if len(holder.asked) != 0 || len(e.files.removed) != 0 {
				t.Errorf("let go of %v, removed %v; want the agent left be", holder.asked, e.files.removed)
			}
		})
	}
}

// A judgement that fails is told, so that the person does not wait for one still running; a
// failure to post is not posted again.
func TestSessionCleaner_CleanNowTellsAFailure(t *testing.T) {
	e := newCleanEnv()
	cleaner := e.withHolder(&fakeHolder{err: errors.New("docker down")})
	e.save(existingID, dev.Name, e.now.Add(-1))
	if err := cleaner.CleanNow(context.Background(), existingID, false, person, e.now.Add(time.Minute)); err == nil {
		t.Error("CleanNow() = nil, want the failure")
	}
	if posts := e.notifier.posts[existingID]; len(e.files.removed) != 0 || len(posts) != 1 || !strings.Contains(posts[0], "判定できませんでした") {
		t.Errorf("removed %v, told %q; want kept and the failure told", e.files.removed, posts)
	}

	e = newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	failing := &countingNotifier{err: errors.New("slack down")}
	cleaner = usecase.NewSessionCleaner(usecase.SessionCleanerDeps{
		Agents: fakeRegistry{dev.Name: dev}, Sessions: e.sessions, Credentials: &fakeIssuer{},
		Workspaces: e.work, Branches: e.branches, Files: e.files, Notifier: failing,
	}, retention, func() time.Time { return e.now })
	if err := cleaner.CleanNow(context.Background(), existingID, false, person, e.now.Add(time.Minute)); err == nil || failing.calls != 1 {
		t.Errorf("CleanNow() = %v after %d posts, want the failure after one", err, failing.calls)
	}
}

type countingNotifier struct {
	err   error
	calls int
}

func (f *countingNotifier) Notify(context.Context, domain.Session, string) error {
	f.calls++
	return f.err
}

// Taken by another cleaning meanwhile, the session is gone: nothing contradicts that.
func TestSessionCleaner_CleanNowAfterAnotherCleaning(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	e.sessions.onLock = func(id domain.SessionID) {
		e.sessions.onLock = nil
		delete(e.sessions.saved, id)
	}
	if err := e.cleaner.CleanNow(context.Background(), existingID, false, person, e.now.Add(time.Minute)); err != nil || len(e.notifier.posts) != 0 {
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
			if err := e.cleaner.CleanNow(context.Background(), existingID, false, person, e.now.Add(time.Minute)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if posts := e.notifier.posts[existingID]; len(posts) != 1 || !strings.Contains(posts[0], "片付けました") {
		t.Errorf("told %q, want only that it was cleaned", posts)
	}
}

// Forced, it lets go only of what the thread was told: a reason found since, before the reaction
// or under the lock, keeps the session and is told.
func TestSessionCleaner_CleanNowForcedKeepsWhatWasNotTold(t *testing.T) {
	for _, underLock := range []bool{false, true} {
		e := newCleanEnv()
		e.save(existingID, dev.Name, e.now.Add(-1))
		e.work[existingID] = usecase.WorkspaceState{Changes: true}
		if err := e.cleaner.CleanNow(context.Background(), existingID, false, person, e.now); err != nil {
			t.Fatal(err)
		}
		more := func() { e.work[existingID] = usecase.WorkspaceState{Changes: true, Stash: true} }
		if underLock {
			e.sessions.onLock = func(domain.SessionID) { e.sessions.onLock = nil; more() }
		} else {
			more()
		}
		if err := e.cleaner.CleanNow(context.Background(), existingID, true, person, e.now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		if len(e.files.removed) != 0 {
			t.Errorf("under the lock %v: removed with a stash the thread was not told of", underLock)
		}
		// Found before the reaction or under the lock, the stash is told; once the person has seen
		// it on their :wastebasket:, it can be forced.
		if posts := e.notifier.posts[existingID]; !strings.Contains(posts[len(posts)-1], "stash") {
			t.Errorf("under the lock %v: told %q, want the stash told", underLock, posts[len(posts)-1])
		}
		e.sessions.onLock = nil
		e.now = e.now.Add(time.Hour)
		if err := e.cleaner.CleanNow(context.Background(), existingID, false, person, e.now); err != nil {
			t.Fatal(err)
		}
		if err := e.cleaner.CleanNow(context.Background(), existingID, true, person, e.now.Add(time.Minute)); err != nil || len(e.files.removed) == 0 {
			t.Errorf("under the lock %v: forced once told of the stash = %v, removed %v; want removed", underLock, err, e.files.removed)
		}
	}
}

// The cleaning by retention and one asked for run one after the other: the later finds the session
// gone and tells nothing.
func TestSessionCleaner_CleanNowWhileCleaningByRetention(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-2*retention))
	e.branches.remote["acp/0f8fad5b"] = merged("acp/0f8fad5b")
	e.sessions.exclusive = true
	removing := make(chan struct{})
	var once sync.Once
	e.branches.onRemove = func() {
		once.Do(func() {
			close(removing)
			time.Sleep(100 * time.Millisecond)
		})
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		if _, err := e.cleaner.Clean(context.Background()); err != nil {
			t.Error(err)
		}
	})
	<-removing
	if err := e.cleaner.CleanNow(context.Background(), existingID, false, person, e.now.Add(time.Minute)); err != nil {
		t.Error(err)
	}
	wg.Wait()
	if posts := e.notifier.posts[existingID]; len(posts) != 1 || !strings.Contains(posts[0], "片付けました") {
		t.Errorf("told %q, want only that it was cleaned", posts)
	}
}

// The :fire: is weighed against the time the person was told on the chat's clock, which the
// reactions carry, not on the control plane's, which may be behind or ahead of it.
func TestSessionCleaner_CleanNowForcedOnTheChatsClock(t *testing.T) {
	for name, tt := range map[string]struct {
		ahead      time.Duration
		after      time.Duration
		wantForced bool
	}{
		"a burst, with the control plane behind":      {ahead: -time.Minute, after: 2 * time.Second},
		"a while after, with the control plane ahead": {ahead: 2 * time.Hour, after: time.Minute, wantForced: true},
	} {
		t.Run(name, func(t *testing.T) {
			e := newCleanEnv()
			e.save(existingID, dev.Name, e.now.Add(-1))
			e.work[existingID] = usecase.WorkspaceState{Changes: true}
			chat := e.now.Add(-tt.ahead)
			if err := e.cleaner.CleanNow(context.Background(), existingID, false, person, chat); err != nil {
				t.Fatal(err)
			}
			e.now = e.now.Add(tt.after)
			if err := e.cleaner.CleanNow(context.Background(), existingID, true, person, chat.Add(tt.after)); err != nil {
				t.Fatal(err)
			}
			if forced := len(e.files.removed) > 0; forced != tt.wantForced {
				t.Errorf("forced %v, want %v", forced, tt.wantForced)
			}
		})
	}
}

// A :fire: within moments of the notice, as in a burst of reactions, is not taken as read.
func TestSessionCleaner_CleanNowForcedRightAfterTheNotice(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	e.work[existingID] = usecase.WorkspaceState{Changes: true}
	if err := e.cleaner.CleanNow(context.Background(), existingID, false, person, e.now); err != nil {
		t.Fatal(err)
	}
	if err := e.cleaner.CleanNow(context.Background(), existingID, true, person, e.now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(e.files.removed) != 0 {
		t.Error("removed by a :fire: put right after the notice")
	}
}

// A branch others pushed to since the thread was told is not what was told: forcing tells again.
func TestSessionCleaner_CleanNowForcedKeepsABranchPushedSinceTold(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	e.branches.remote["acp/0f8fad5b"] = unmerged("acp/0f8fad5b")
	if err := e.cleaner.CleanNow(context.Background(), existingID, false, person, e.now); err != nil {
		t.Fatal(err)
	}
	b := unmerged("acp/0f8fad5b")
	b.Tip = "pushed-by-someone"
	b.Commits = map[string]usecase.CommitPlaces{b.Tip: {InBranch: true}}
	e.branches.remote["acp/0f8fad5b"] = b
	if err := e.cleaner.CleanNow(context.Background(), existingID, true, person, e.now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(e.files.removed) != 0 || len(e.branches.removed) != 0 {
		t.Errorf("removed files %v, branches %v; want the branch others pushed to kept", e.files.removed, e.branches.removed)
	}
}

// :fire: forces only for the person who put :wastebasket: and was told the reasons: not for
// someone else, nor after the cleanup told them of itself.
func TestSessionCleaner_CleanNowForcedOnlyByWhoAsked(t *testing.T) {
	other := domain.Requester{Platform: "slack", ID: "U_BOB"}
	for name, tell := range map[string]func(e *cleanEnv){
		"told on another's :wastebasket:": func(e *cleanEnv) {
			if err := e.cleaner.CleanNow(context.Background(), existingID, false, other, e.now); err != nil {
				t.Fatal(err)
			}
		},
		"told by the retention's notice after one's own :wastebasket:": func(e *cleanEnv) {
			if err := e.cleaner.CleanNow(context.Background(), existingID, false, person, e.now); err != nil {
				t.Fatal(err)
			}
			s := e.sessions.saved[existingID]
			s.LastUsedAt = e.now.Add(-2 * retention)
			s.KeptNotifiedAt = e.now.Add(-2 * retention)
			e.sessions.saved[existingID] = s
			if _, err := e.cleaner.Clean(context.Background()); err != nil {
				t.Fatal(err)
			}
		},
		"told on one's own :wastebasket: long before": func(e *cleanEnv) {
			if err := e.cleaner.CleanNow(context.Background(), existingID, false, person, e.now.Add(-2*time.Hour)); err != nil {
				t.Fatal(err)
			}
			s := e.sessions.saved[existingID]
			s.LastUsedAt = e.now.Add(-3 * time.Hour)
			s.KeptNotifiedAt = e.now.Add(-2 * time.Hour)
			e.sessions.saved[existingID] = s
		},
		"told on a :fire: of one's own": func(e *cleanEnv) {
			if err := e.cleaner.CleanNow(context.Background(), existingID, true, person, e.now); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newCleanEnv()
			e.save(existingID, dev.Name, e.now.Add(-1))
			e.work[existingID] = usecase.WorkspaceState{Changes: true}
			tell(e)
			if err := e.cleaner.CleanNow(context.Background(), existingID, true, person, e.now.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			posts := e.notifier.posts[existingID]
			if len(e.files.removed) != 0 || !strings.Contains(posts[len(posts)-1], "まず :fire: を外し") {
				t.Errorf("removed %v, told %q; want kept and told to put :wastebasket: first", e.files.removed, posts[len(posts)-1])
			}
		})
	}
}

// A :fire: stopped only by an open pull request leaves the person able to force once it is gone.
func TestSessionCleaner_CleanNowForcedAgainAfterThePullRequest(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	e.work[existingID] = usecase.WorkspaceState{Changes: true}
	b := merged("acp/0f8fad5b")
	b.OpenHead = []usecase.PullRequest{{Number: 7}}
	e.branches.remote["acp/0f8fad5b"] = b
	if err := e.cleaner.CleanNow(context.Background(), existingID, false, person, e.now); err != nil {
		t.Fatal(err)
	}
	if err := e.cleaner.CleanNow(context.Background(), existingID, true, person, e.now.Add(time.Minute)); err != nil || len(e.files.removed) != 0 {
		t.Fatalf("forced with the pull request open = %v, removed %v; want kept", err, e.files.removed)
	}
	// Gone, the pull request no longer keeps it: :fire: put again forces, with no new :wastebasket:.
	e.branches.remote["acp/0f8fad5b"] = merged("acp/0f8fad5b")
	if err := e.cleaner.CleanNow(context.Background(), existingID, true, person, e.now.Add(2*time.Minute)); err != nil || len(e.files.removed) == 0 {
		t.Errorf("forced again once the pull request was gone = %v, removed %v; want removed", err, e.files.removed)
	}
}
