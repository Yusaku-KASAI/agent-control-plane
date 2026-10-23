package usecase_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// A person asks in the thread: the session is judged now, however recently it was used.
func TestSessionCleaner_CleanNow(t *testing.T) {
	recently := func(e *cleanEnv) { e.save(existingID, dev.Name, e.now.Add(-1)) }
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
		"forced, the work is let go": {
			arrange:     func(e *cleanEnv) { e.work[existingID] = usecase.WorkspaceState{Changes: true} },
			force:       true,
			wantRemoved: true, wantText: "承知のうえで捨てたもの",
		},
		"forced, an open pull request still keeps it": {
			arrange: func(e *cleanEnv) {
				b := unmerged("acp/0f8fad5b")
				b.OpenHead = []usecase.PullRequest{pull}
				e.branches.remote["acp/0f8fad5b"] = b
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

			if err := e.cleaner.CleanNow(context.Background(), existingID, tt.force); err != nil {
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
		s.LastUsedAt = e.now.Add(1)
		e.sessions.saved[id] = s
	}
	if err := e.cleaner.CleanNow(context.Background(), existingID, false); err != nil {
		t.Fatal(err)
	}
	if posts := e.notifier.posts[existingID]; len(e.files.removed) != 0 || len(posts) != 1 || !strings.Contains(posts[0], "使われた") {
		t.Errorf("removed %v, told %q; want it kept and the thread told", e.files.removed, posts)
	}
}

// A thread with no session, such as one already cleaned, is left alone.
func TestSessionCleaner_CleanNowWithoutSession(t *testing.T) {
	e := newCleanEnv()
	if err := e.cleaner.CleanNow(context.Background(), existingID, false); err != nil || len(e.notifier.posts) != 0 {
		t.Errorf("CleanNow() = %v, told %v; want nothing", err, e.notifier.posts)
	}
}
