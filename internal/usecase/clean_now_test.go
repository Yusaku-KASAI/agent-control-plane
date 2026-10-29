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

// channel is where the reactions are put: the thread's, as the bot hears them.
var channel = domain.Conversation{Platform: cleanThread.Platform, Workspace: cleanThread.Workspace, Channel: cleanThread.Channel}

// forceOn puts the person's :fire: on the last post to the thread, as on the notice of what keeps
// the session.
func forceOn(c *usecase.SessionCleaner, e *cleanEnv) error {
	return c.ForceClean(context.Background(), dev.Name, person, channel, e.notifier.lastMessage(existingID))
}

// A person asks in the thread: the session is judged now, however recently it was used.
func TestSessionCleaner_CleanNow(t *testing.T) {
	recently := func(e *cleanEnv) { e.save(existingID, dev.Name, e.now.Add(-1)) }
	// told has the thread told of the reasons, as by :wastebasket:, before the test's reaction.
	told := func(e *cleanEnv) {
		if err := e.cleaner.CleanNow(context.Background(), existingID, person); err != nil {
			panic(err)
		}
		e.notifier.posts = map[domain.SessionID][]string{}
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
			wantText: "この知らせに :fire:",
		},
		"forced on the notice, the work is let go": {
			arrange: func(e *cleanEnv) {
				e.work[existingID] = usecase.WorkspaceState{Changes: true}
				told(e)
			},
			force:       true,
			wantRemoved: true, wantText: "承知のうえで捨てたもの",
		},
		"forced after being used since told, it tells again": {
			arrange: func(e *cleanEnv) {
				e.work[existingID] = usecase.WorkspaceState{Changes: true}
				told(e)
				s := e.sessions.saved[existingID]
				s.LastUsedAt = e.now.Add(time.Second)
				e.sessions.saved[existingID] = s
			},
			force:    true,
			wantText: "この知らせに :fire:",
		},
		"forced once nothing keeps it any more, it is removed": {
			arrange: func(e *cleanEnv) {
				e.work[existingID] = usecase.WorkspaceState{Changes: true}
				told(e)
				e.work[existingID] = usecase.WorkspaceState{}
			},
			force:       true,
			wantRemoved: true, wantText: "片付けました",
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
			e.now = e.now.Add(time.Minute)

			var err error
			if tt.force {
				err = forceOn(e.cleaner, e)
			} else {
				err = e.cleaner.CleanNow(context.Background(), existingID, person)
			}
			if err != nil {
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

// A :fire: anywhere but on a notice the person was told within the hour, by this agent, does
// nothing and tells nothing: it may be praise.
func TestSessionCleaner_ForceCleanOnlyOnTheNotice(t *testing.T) {
	other := domain.Requester{Platform: "slack", ID: "U_BOB"}
	for name, tt := range map[string]struct {
		by      domain.Requester
		agent   domain.AgentName
		where   domain.Conversation
		message func(e *cleanEnv, notice string) string
		later   time.Duration
		restart bool
	}{
		"on another message":           {by: person, agent: dev.Name, message: func(*cleanEnv, string) string { return "1700000000.999999" }},
		"by someone else":              {by: other, agent: dev.Name, message: func(_ *cleanEnv, n string) string { return n }},
		"heard by another agent's bot": {by: person, agent: "reviewer", message: func(_ *cleanEnv, n string) string { return n }},
		"in another channel":           {by: person, agent: dev.Name, where: domain.Conversation{Platform: "slack", Workspace: "T1", Channel: "C2"}, message: func(_ *cleanEnv, n string) string { return n }},
		"in another workspace":         {by: person, agent: dev.Name, where: domain.Conversation{Platform: "slack", Workspace: "T2", Channel: "C1"}, message: func(_ *cleanEnv, n string) string { return n }},
		"past the hour":                {by: person, agent: dev.Name, message: func(_ *cleanEnv, n string) string { return n }, later: time.Hour + time.Second},
		"after a restart":              {by: person, agent: dev.Name, message: func(_ *cleanEnv, n string) string { return n }, restart: true},
		"on a notice told again since": {by: person, agent: dev.Name, message: func(e *cleanEnv, n string) string {
			if err := e.cleaner.CleanNow(context.Background(), existingID, person); err != nil {
				panic(err)
			}
			return n
		}},
	} {
		t.Run(name, func(t *testing.T) {
			e := newCleanEnv()
			e.save(existingID, dev.Name, e.now.Add(-1))
			e.work[existingID] = usecase.WorkspaceState{Changes: true}
			if err := e.cleaner.CleanNow(context.Background(), existingID, person); err != nil {
				t.Fatal(err)
			}
			message := tt.message(e, e.notifier.messages[existingID][0])
			told := len(e.notifier.posts[existingID])
			e.now = e.now.Add(time.Minute + tt.later)
			cleaner := e.cleaner
			if tt.restart {
				cleaner = usecase.NewSessionCleaner(usecase.SessionCleanerDeps{
					Agents: fakeRegistry{dev.Name: dev}, Sessions: e.sessions, Credentials: &fakeIssuer{},
					Workspaces: e.work, Branches: e.branches, Files: e.files, Notifier: e.notifier,
				}, retention, func() time.Time { return e.now })
			}
			where := channel
			if tt.where != (domain.Conversation{}) {
				where = tt.where
			}
			if err := cleaner.ForceClean(context.Background(), tt.agent, tt.by, where, message); err != nil {
				t.Fatal(err)
			}
			if posts := e.notifier.posts[existingID]; len(e.files.removed) != 0 || len(posts) != told {
				t.Errorf("removed %v, told %q; want nothing done", e.files.removed, posts[told:])
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
	if err := e.cleaner.CleanNow(context.Background(), existingID, person); err != nil {
		t.Fatal(err)
	}
	if posts := e.notifier.posts[existingID]; len(e.files.removed) != 0 || len(posts) != 1 || !strings.Contains(posts[0], "使われた") {
		t.Errorf("removed %v, told %q; want it kept and the thread told", e.files.removed, posts)
	}
}

// A thread with no session, such as one already cleaned, is left alone.
func TestSessionCleaner_CleanNowWithoutSession(t *testing.T) {
	e := newCleanEnv()
	if err := e.cleaner.CleanNow(context.Background(), existingID, person); err != nil || len(e.notifier.posts) != 0 {
		t.Errorf("CleanNow() = %v, told %v; want nothing", err, e.notifier.posts)
	}
}

type fakeHolder struct {
	idle  bool
	err   error
	asked []domain.SessionID
	// onRelease runs as an idle agent is let go of.
	onRelease func()
}

func (f *fakeHolder) Release(_ context.Context, id domain.SessionID) (bool, error) {
	f.asked = append(f.asked, id)
	if f.idle && f.onRelease != nil {
		f.onRelease()
	}
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

		if err := cleaner.CleanNow(context.Background(), existingID, person); err != nil {
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

// A session that is not to be removed leaves its agent be: a :wastebasket: on kept work, or a
// :fire: that tells again, does not stop the container the next request would use.
func TestSessionCleaner_CleanNowLetsGoOnlyToRemove(t *testing.T) {
	for name, tt := range map[string]struct {
		force bool
		work  usecase.WorkspaceState
	}{
		"kept for its work":   {work: usecase.WorkspaceState{Changes: true}},
		"told again on force": {force: true, work: usecase.WorkspaceState{Changes: true, Stash: true}},
	} {
		t.Run(name, func(t *testing.T) {
			e := newCleanEnv()
			holder := &fakeHolder{idle: true}
			cleaner := e.withHolder(holder)
			e.save(existingID, dev.Name, e.now.Add(-1))
			e.work[existingID] = usecase.WorkspaceState{Changes: true}
			if err := cleaner.CleanNow(context.Background(), existingID, person); err != nil {
				t.Fatal(err)
			}
			e.work[existingID] = tt.work
			if tt.force {
				if err := forceOn(cleaner, e); err != nil {
					t.Fatal(err)
				}
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
	if err := cleaner.CleanNow(context.Background(), existingID, person); err == nil {
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
	if err := cleaner.CleanNow(context.Background(), existingID, person); err == nil || failing.calls != 1 {
		t.Errorf("CleanNow() = %v after %d posts, want the failure after one", err, failing.calls)
	}
}

type countingNotifier struct {
	err   error
	calls int
}

func (f *countingNotifier) Notify(context.Context, domain.Session, string) (string, error) {
	f.calls++
	return "", f.err
}

func (f *countingNotifier) MarkCleaned(context.Context, domain.Session) error { return nil }

// Taken by another cleaning meanwhile, the session is gone: nothing contradicts that.
func TestSessionCleaner_CleanNowAfterAnotherCleaning(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	e.sessions.onLock = func(id domain.SessionID) {
		e.sessions.onLock = nil
		delete(e.sessions.saved, id)
	}
	if err := e.cleaner.CleanNow(context.Background(), existingID, person); err != nil || len(e.notifier.posts) != 0 {
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
			if err := e.cleaner.CleanNow(context.Background(), existingID, person); err != nil {
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
		if err := e.cleaner.CleanNow(context.Background(), existingID, person); err != nil {
			t.Fatal(err)
		}
		more := func() { e.work[existingID] = usecase.WorkspaceState{Changes: true, Stash: true} }
		if underLock {
			e.sessions.onLock = func(domain.SessionID) { e.sessions.onLock = nil; more() }
		} else {
			more()
		}
		e.now = e.now.Add(time.Minute)
		if err := forceOn(e.cleaner, e); err != nil {
			t.Fatal(err)
		}
		if len(e.files.removed) != 0 {
			t.Errorf("under the lock %v: removed with a stash the thread was not told of", underLock)
		}
		// Found before the reaction or under the lock, the stash is told in a new notice; a :fire:
		// on that one forces.
		if posts := e.notifier.posts[existingID]; !strings.Contains(posts[len(posts)-1], "stash") ||
			!strings.Contains(posts[len(posts)-1], "この知らせに :fire:") {
			t.Errorf("under the lock %v: told %q, want the stash told in a notice to force", underLock, posts[len(posts)-1])
		}
		e.sessions.onLock = nil
		e.now = e.now.Add(time.Minute)
		if err := forceOn(e.cleaner, e); err != nil || len(e.files.removed) == 0 {
			t.Errorf("under the lock %v: forced on the new notice = %v, removed %v; want removed", underLock, err, e.files.removed)
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
	if err := e.cleaner.CleanNow(context.Background(), existingID, person); err != nil {
		t.Error(err)
	}
	wg.Wait()
	if posts := e.notifier.posts[existingID]; len(posts) != 1 || !strings.Contains(posts[0], "片付けました") {
		t.Errorf("told %q, want only that it was cleaned", posts)
	}
}

// Work done while the :wastebasket: is judged, after the session was read and before the notice,
// is not what was told: the :fire: tells again.
func TestSessionCleaner_CleanNowForcedKeepsWorkDoneWhileJudged(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	e.work[existingID] = usecase.WorkspaceState{Changes: true}
	cleaner := usecase.NewSessionCleaner(usecase.SessionCleanerDeps{
		Agents: fakeRegistry{dev.Name: dev}, Sessions: e.sessions, Credentials: &usingIssuer{e: e},
		Workspaces: e.work, Branches: e.branches, Files: e.files, Notifier: e.notifier,
	}, retention, func() time.Time { return e.now })
	if err := cleaner.CleanNow(context.Background(), existingID, person); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(time.Minute)
	if err := forceOn(cleaner, e); err != nil {
		t.Fatal(err)
	}
	if len(e.files.removed) != 0 {
		t.Error("removed the work done while the :wastebasket: was judged")
	}
}

// Used while the :fire: is judged, the session is kept even when the clock was set back meanwhile.
func TestSessionCleaner_CleanNowForcedKeepsWorkDoneWithTheClockSetBack(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-time.Hour))
	e.work[existingID] = usecase.WorkspaceState{Changes: true}
	if err := e.cleaner.CleanNow(context.Background(), existingID, person); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(time.Minute)
	e.sessions.onLock = func(id domain.SessionID) {
		e.sessions.onLock = nil
		s := e.sessions.saved[id]
		s.LastUsedAt = s.LastUsedAt.Add(-time.Second)
		e.sessions.saved[id] = s
	}
	if err := forceOn(e.cleaner, e); err != nil {
		t.Fatal(err)
	}
	if len(e.files.removed) != 0 {
		t.Error("removed a session used while the :fire: was judged")
	}
}

// What is discarded is told as judged under the lock, where the removal is decided.
func TestSessionCleaner_CleanNowForcedTellsWhatItDiscarded(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	e.work[existingID] = usecase.WorkspaceState{Changes: true, Stash: true}
	if err := e.cleaner.CleanNow(context.Background(), existingID, person); err != nil {
		t.Fatal(err)
	}
	e.work[existingID] = usecase.WorkspaceState{Changes: true}
	e.sessions.onLock = func(domain.SessionID) {
		e.sessions.onLock = nil
		e.work[existingID] = usecase.WorkspaceState{Changes: true, Stash: true}
	}
	e.now = e.now.Add(time.Minute)
	if err := forceOn(e.cleaner, e); err != nil || len(e.files.removed) == 0 {
		t.Fatalf("forced = %v, removed %v; want removed", err, e.files.removed)
	}
	if posts := e.notifier.posts[existingID]; !strings.Contains(posts[len(posts)-1], "stash") {
		t.Errorf("told %q, want the stash among what was discarded", posts[len(posts)-1])
	}
}

// usingIssuer has the agent use the session, a second before the notice, while the first
// cleaning is judged.
type usingIssuer struct {
	e    *cleanEnv
	used bool
}

func (u *usingIssuer) Issue(context.Context, domain.Agent) (domain.Credentials, error) {
	if u.used {
		return domain.Credentials{GitHubToken: "token"}, nil
	}
	u.used = true
	s := u.e.sessions.saved[existingID]
	s.LastUsedAt = u.e.now
	u.e.sessions.saved[existingID] = s
	u.e.now = u.e.now.Add(time.Second)
	return domain.Credentials{GitHubToken: "token"}, nil
}

// An agent waiting idle holds the session's lock: the :wastebasket: does not let go of it, nor
// need to, for the person to force once told.
func TestSessionCleaner_CleanNowForcedWhileTheAgentWaits(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	e.work[existingID] = usecase.WorkspaceState{Changes: true}
	e.sessions.held = map[domain.SessionID]bool{existingID: true}
	holder := &fakeHolder{idle: true, onRelease: func() { e.sessions.held = nil }}
	cleaner := e.withHolder(holder)
	if err := cleaner.CleanNow(context.Background(), existingID, person); err != nil {
		t.Fatal(err)
	}
	if len(holder.asked) != 0 {
		t.Errorf("let go of %v on :wastebasket:, want the agent left be", holder.asked)
	}
	e.now = e.now.Add(time.Minute)
	if err := forceOn(cleaner, e); err != nil || len(e.files.removed) == 0 {
		t.Errorf("forced = %v, removed %v; want removed", err, e.files.removed)
	}
}

// A branch others pushed to since the thread was told is not what was told: forcing tells again.
func TestSessionCleaner_CleanNowForcedKeepsABranchPushedSinceTold(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	e.branches.remote["acp/0f8fad5b"] = unmerged("acp/0f8fad5b")
	if err := e.cleaner.CleanNow(context.Background(), existingID, person); err != nil {
		t.Fatal(err)
	}
	b := unmerged("acp/0f8fad5b")
	b.Tip = "pushed-by-someone"
	b.Commits = map[string]usecase.CommitPlaces{b.Tip: {InBranch: true}}
	e.branches.remote["acp/0f8fad5b"] = b
	if err := forceOn(e.cleaner, e); err != nil {
		t.Fatal(err)
	}
	if len(e.files.removed) != 0 || len(e.branches.removed) != 0 {
		t.Errorf("removed files %v, branches %v; want the branch others pushed to kept", e.files.removed, e.branches.removed)
	}
}

// The retention's notice after one's own :wastebasket: forgets the notice: a :fire: on either does
// nothing.
func TestSessionCleaner_ForceCleanAfterTheRetentionsNotice(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	e.work[existingID] = usecase.WorkspaceState{Changes: true}
	if err := e.cleaner.CleanNow(context.Background(), existingID, person); err != nil {
		t.Fatal(err)
	}
	notice := e.notifier.lastMessage(existingID)
	s := e.sessions.saved[existingID]
	s.LastUsedAt = e.now.Add(-2 * retention)
	s.KeptNotifiedAt = e.now.Add(-2 * retention)
	e.sessions.saved[existingID] = s
	if _, err := e.cleaner.Clean(context.Background()); err != nil {
		t.Fatal(err)
	}
	told := len(e.notifier.posts[existingID])
	for _, message := range []string{notice, e.notifier.lastMessage(existingID)} {
		if err := e.cleaner.ForceClean(context.Background(), dev.Name, person, channel, message); err != nil {
			t.Fatal(err)
		}
	}
	if posts := e.notifier.posts[existingID]; len(e.files.removed) != 0 || len(posts) != told {
		t.Errorf("removed %v, told %q; want nothing done", e.files.removed, posts[told:])
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
	if err := e.cleaner.CleanNow(context.Background(), existingID, person); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(time.Minute)
	if err := forceOn(e.cleaner, e); err != nil || len(e.files.removed) != 0 {
		t.Fatalf("forced with the pull request open = %v, removed %v; want kept", err, e.files.removed)
	}
	// The :fire: put again on the first notice no longer counts: the one told again says so.
	if posts := e.notifier.posts[existingID]; !strings.Contains(posts[len(posts)-1], "この知らせに :fire:") {
		t.Errorf("told %q, want the :fire: asked on the notice told again", posts[len(posts)-1])
	}
	// Gone, the pull request no longer keeps it: :fire: on the notice told again forces, with no new
	// :wastebasket:.
	e.branches.remote["acp/0f8fad5b"] = merged("acp/0f8fad5b")
	e.now = e.now.Add(time.Minute)
	if err := forceOn(e.cleaner, e); err != nil || len(e.files.removed) == 0 {
		t.Errorf("forced again once the pull request was gone = %v, removed %v; want removed", err, e.files.removed)
	}
}

// A session removed, by retention or as asked, is marked cleaned where it started, under its lock:
// a session made after in the thread, which takes the mark off, finds it there. One kept is not.
func TestSessionCleaner_MarksTheRemoved(t *testing.T) {
	for name, tt := range map[string]struct {
		lastUsed   time.Duration
		work       usecase.WorkspaceState
		now        bool
		wantMarked bool
	}{
		"removed by retention": {lastUsed: -2 * retention, wantMarked: true},
		"kept by retention":    {lastUsed: -2 * retention, work: usecase.WorkspaceState{Changes: true}},
		"removed as asked":     {lastUsed: -1, now: true, wantMarked: true},
		"kept as asked":        {lastUsed: -1, now: true, work: usecase.WorkspaceState{Changes: true}},
	} {
		t.Run(name, func(t *testing.T) {
			e := newCleanEnv()
			e.save(existingID, dev.Name, e.now.Add(tt.lastUsed))
			e.work[existingID] = tt.work
			e.notifier.onMark = func(id domain.SessionID) {
				e.sessions.mu.Lock()
				defer e.sessions.mu.Unlock()
				if e.sessions.locks[id] == 0 {
					t.Error("marked without the session's lock")
				}
			}
			var err error
			if tt.now {
				err = e.cleaner.CleanNow(context.Background(), existingID, person)
			} else {
				_, err = e.cleaner.Clean(context.Background())
			}
			if err != nil {
				t.Fatal(err)
			}
			if marked := len(e.notifier.marked) == 1 && e.notifier.marked[0] == existingID; marked != tt.wantMarked {
				t.Errorf("marked %v, want marked %v", e.notifier.marked, tt.wantMarked)
			}
		})
	}
}

// Failing to mark a session removed as asked is not told as a failure to judge: it was removed,
// and the thread was told so.
func TestSessionCleaner_CleanNowFailingToMark(t *testing.T) {
	e := newCleanEnv()
	e.save(existingID, dev.Name, e.now.Add(-1))
	e.notifier.markErr = errors.New("slack down")
	if err := e.cleaner.CleanNow(context.Background(), existingID, person); err == nil {
		t.Error("CleanNow() = nil, want the failure to mark")
	}
	if posts := e.notifier.posts[existingID]; len(posts) != 1 || !strings.Contains(posts[0], "片付けました") {
		t.Errorf("told %q, want only that it was cleaned", posts)
	}
}
