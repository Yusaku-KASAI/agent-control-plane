package gitworkspace_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/gitworkspace"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

const sessionID domain.SessionID = "0f8fad5b-d9cb-469f-a165-70867728950e"

// fixture builds a remote whose main is ahead of the source checkout, and a source that has a
// feature branch checked out plus an uncommitted file.
type fixture struct {
	remote, source         string
	remoteMain, sourceHEAD string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	seed := filepath.Join(base, "seed")
	source := filepath.Join(base, "repo")

	git(t, "", "init", "--quiet", "--bare", "-b", "main", remote)
	git(t, "", "clone", "--quiet", remote, seed)
	commit(t, seed, "README.md", "v1")
	git(t, seed, "push", "--quiet", "origin", "HEAD:main")

	git(t, "", "clone", "--quiet", remote, source)
	git(t, source, "switch", "--quiet", "-c", "feature")
	commit(t, source, "feature.txt", "local work")
	if err := os.WriteFile(filepath.Join(source, "dirty.txt"), []byte("uncommitted"), 0o644); err != nil {
		t.Fatal(err)
	}

	commit(t, seed, "README.md", "v2")
	git(t, seed, "push", "--quiet", "origin", "HEAD:main")

	return fixture{
		remote:     remote,
		source:     source,
		remoteMain: gitOutput(t, seed, "rev-parse", "HEAD"),
		sourceHEAD: gitOutput(t, source, "rev-parse", "HEAD"),
	}
}

func TestProvider_Prepare(t *testing.T) {
	tests := []struct {
		name      string
		creds     domain.Credentials
		wantStart func(fixture) string
	}{
		{
			name:      "with credentials, starts from the remote default branch",
			creds:     domain.Credentials{GitHubToken: "token"},
			wantStart: func(f fixture) string { return f.remoteMain },
		},
		{
			name:      "without credentials, starts from the target's HEAD",
			wantStart: func(f fixture) string { return f.sourceHEAD },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			root := t.TempDir()
			agent := domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: f.source, Image: "img"}}

			ws, err := gitworkspace.NewProvider(root).Prepare(context.Background(), agent, sessionID, tt.creds)
			if err != nil {
				t.Fatalf("Prepare() error = %v", err)
			}
			if want := filepath.Join(root, string(sessionID), "workspace", "repo"); ws.Path != want {
				t.Errorf("Path = %q, want %q", ws.Path, want)
			}
			if got := gitOutput(t, ws.Path, "branch", "--show-current"); got != "acp/0f8fad5b" {
				t.Errorf("branch = %q, want acp/0f8fad5b", got)
			}
			if got, want := gitOutput(t, ws.Path, "rev-parse", "HEAD"), tt.wantStart(f); got != want {
				t.Errorf("HEAD = %s, want %s", got, want)
			}
			if got := gitOutput(t, ws.Path, "remote", "get-url", "origin"); got != f.remote {
				t.Errorf("origin = %q, want the target's origin %q", got, f.remote)
			}
			if _, err := os.Stat(filepath.Join(ws.Path, "dirty.txt")); !os.IsNotExist(err) {
				t.Errorf("uncommitted file was copied (stat err = %v)", err)
			}
		})
	}
}

func TestProvider_Prepare_CreditsCoAuthors(t *testing.T) {
	f := newFixture(t)
	agent := domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: f.source, Image: "img"}}
	ws, err := gitworkspace.NewProvider(t.TempDir()).Prepare(context.Background(), agent, sessionID, domain.Credentials{})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	coAuthors := filepath.Join(t.TempDir(), "co-authors")
	if err := os.WriteFile(coAuthors, []byte("Alice <alice@example.com>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ACP_CO_AUTHORS_FILE", coAuthors)
	commit(t, ws.Path, "change.txt", "agent work")

	msg := gitOutput(t, ws.Path, "log", "-1", "--format=%B")
	if !strings.Contains(msg, "Co-authored-by: Alice <alice@example.com>") {
		t.Errorf("commit message = %q, want the co-author trailer", msg)
	}
}

func TestProvider_Prepare_NotARepository(t *testing.T) {
	agent := domain.Agent{Name: "a", Role: "developer", Target: domain.Target{Path: t.TempDir(), Image: "img"}}
	if _, err := gitworkspace.NewProvider(t.TempDir()).Prepare(context.Background(), agent, sessionID, domain.Credentials{}); err == nil {
		t.Fatal("Prepare() error = nil, want error")
	}
}

func commit(t *testing.T, dir, file, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", file)
	git(t, dir, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "--quiet", "-m", content)
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	gitOutput(t, dir, args...)
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestOrigin(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	if got, err := gitworkspace.Origin(ctx, f.source); err != nil || got != f.remote {
		t.Errorf("Origin(clone) = %q, %v; want %q", got, err, f.remote)
	}
	if got, err := gitworkspace.Origin(ctx, f.remote); err != nil || got != "" {
		t.Errorf("Origin(no origin) = %q, %v; want empty", got, err)
	}
	if _, err := gitworkspace.Origin(ctx, t.TempDir()); err == nil {
		t.Error("Origin(not a repository) succeeded")
	}
}

func TestProvider_State_Branches(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	p := gitworkspace.NewProvider(t.TempDir())
	agent := domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: f.source, Image: "img"}}
	ws, err := p.Prepare(ctx, agent, sessionID, domain.Credentials{GitHubToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	type local struct {
		name     string
		unpushed bool
	}
	check := func(wantPushed []string, wantLocal ...local) {
		t.Helper()
		got, err := p.State(ctx, ws.Path)
		if err != nil {
			t.Fatal(err)
		}
		var pushed []string
		for _, b := range got.Pushed {
			if want := gitOutput(t, ws.Path, "rev-parse", "refs/remotes/origin/"+b.Name); b.Tip != want {
				t.Errorf("tip of pushed %s = %q, want %q", b.Name, b.Tip, want)
			}
			pushed = append(pushed, b.Name)
		}
		if !slices.Equal(pushed, wantPushed) {
			t.Errorf("Pushed = %v, want %v", pushed, wantPushed)
		}
		var locals []local
		for _, b := range got.Local {
			if rev := "refs/heads/" + b.Name; b.Name == "HEAD" && b.Tip != gitOutput(t, ws.Path, "rev-parse", "HEAD") ||
				b.Name != "HEAD" && b.Tip != gitOutput(t, ws.Path, "rev-parse", rev) {
				t.Errorf("tip of %s = %q", b.Name, b.Tip)
			}
			locals = append(locals, local{b.Name, b.Unpushed})
		}
		if !slices.Equal(locals, wantLocal) {
			t.Errorf("Local = %+v, want %+v", locals, wantLocal)
		}
	}

	// The clone's branch, feature, holds a commit origin lacks, but it is the target's. Fetching
	// origin/main is no push.
	check(nil, local{"acp/0f8fad5b", false}, local{"feature", false})

	commit(t, ws.Path, "a.txt", "session work")
	check(nil, local{"acp/0f8fad5b", true}, local{"feature", false})
	git(t, ws.Path, "push", "--quiet", "origin", "acp/0f8fad5b")
	check([]string{"acp/0f8fad5b"}, local{"acp/0f8fad5b", false}, local{"feature", false})

	git(t, ws.Path, "switch", "--quiet", "-c", "scratch")
	commit(t, ws.Path, "b.txt", "docs")
	git(t, ws.Path, "push", "--quiet", "origin", "HEAD:refs/heads/acp/0f8fad5b-docs")
	commit(t, ws.Path, "c.txt", "scratch")
	git(t, ws.Path, "tag", "scratch")
	git(t, ws.Path, "switch", "--quiet", "--detach", "acp/0f8fad5b")
	commit(t, ws.Path, "d.txt", "detached")
	want := []local{{"acp/0f8fad5b", false}, {"feature", false}, {"scratch", true}, {"HEAD", true}}
	check([]string{"acp/0f8fad5b", "acp/0f8fad5b-docs"}, want...)

	// What the session pushed outlives gc, whatever the retention period.
	git(t, ws.Path, "-c", "gc.reflogExpire=now", "-c", "gc.reflogExpireUnreachable=now", "reflog", "expire", "--all")
	check([]string{"acp/0f8fad5b", "acp/0f8fad5b-docs"}, want...)
}

func TestProvider_State_NoRepository(t *testing.T) {
	p := gitworkspace.NewProvider(t.TempDir())
	// A directory inside another repository must not be read as that repository.
	inside := filepath.Join(newFixture(t).source, "sub")
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", filepath.Join(t.TempDir(), "gone"), inside} {
		if got, err := p.State(context.Background(), path); !reflect.DeepEqual(got, usecase.WorkspaceState{}) || err != nil {
			t.Errorf("State(%q) = %+v, %v; want nothing", path, got, err)
		}
	}
}

func TestProvider_State_WithoutCredentials(t *testing.T) {
	f := newFixture(t)
	agent := domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: f.source, Image: "img"}}
	p := gitworkspace.NewProvider(t.TempDir())
	ws, err := p.Prepare(context.Background(), agent, sessionID, domain.Credentials{})
	if err != nil {
		t.Fatal(err)
	}
	// The session branch starts from the target's commit, which the clone's remote-tracking branches hold.
	got, err := p.State(context.Background(), ws.Path)
	if err != nil || len(got.Pushed) != 0 || slices.ContainsFunc(got.Local, func(b usecase.LocalBranch) bool { return b.Unpushed }) {
		t.Errorf("State() = %+v, %v; want nothing pushed or unpushed", got, err)
	}
}

func TestProvider_State_WorkingTree(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	p := gitworkspace.NewProvider(t.TempDir())
	agent := domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: f.source, Image: "img"}}
	ws, err := p.Prepare(ctx, agent, sessionID, domain.Credentials{GitHubToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	check := func(want usecase.WorkspaceState) {
		t.Helper()
		got, err := p.State(ctx, ws.Path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Changes != want.Changes || got.Stash != want.Stash || got.Operation != want.Operation {
			t.Errorf("State() = changes %v, stash %v, operation %q; want %v, %v, %q",
				got.Changes, got.Stash, got.Operation, want.Changes, want.Stash, want.Operation)
		}
	}
	check(usecase.WorkspaceState{})

	if err := os.WriteFile(filepath.Join(ws.Path, "new.txt"), []byte("untracked"), 0o644); err != nil {
		t.Fatal(err)
	}
	check(usecase.WorkspaceState{Changes: true})
	git(t, ws.Path, "add", "new.txt")
	git(t, ws.Path, "-c", "user.name=test", "-c", "user.email=test@example.com", "stash", "--quiet")
	check(usecase.WorkspaceState{Stash: true})
	git(t, ws.Path, "stash", "drop", "--quiet")

	// A merge stopped by a conflict.
	commit(t, ws.Path, "README.md", "session")
	git(t, ws.Path, "switch", "--quiet", "-c", "other", "HEAD~1")
	commit(t, ws.Path, "README.md", "other")
	cmd := exec.Command("git", "-c", "user.name=test", "-c", "user.email=test@example.com", "merge", "--quiet", "acp/0f8fad5b")
	cmd.Dir = ws.Path
	if err := cmd.Run(); err == nil {
		t.Fatal("merge succeeded, want a conflict")
	}
	check(usecase.WorkspaceState{Changes: true, Operation: "merge"})
}

func TestProvider_State_PushedTipIsWhatWasPushed(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	p := gitworkspace.NewProvider(t.TempDir())
	agent := domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: f.source, Image: "img"}}
	ws, err := p.Prepare(ctx, agent, sessionID, domain.Credentials{GitHubToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	commit(t, ws.Path, "a.txt", "session work")
	git(t, ws.Path, "push", "--quiet", "origin", "acp/0f8fad5b")
	pushed := gitOutput(t, ws.Path, "rev-parse", "HEAD")

	// Someone force-pushes the branch away from the session's commit, and the workspace fetches it.
	other := filepath.Join(t.TempDir(), "other")
	git(t, "", "clone", "--quiet", f.remote, other)
	commit(t, other, "b.txt", "someone else")
	git(t, other, "push", "--quiet", "--force", "origin", "HEAD:acp/0f8fad5b")
	git(t, ws.Path, "fetch", "--quiet", "origin")

	got, err := p.State(ctx, ws.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pushed) != 1 || got.Pushed[0].Tip != pushed {
		t.Errorf("Pushed = %+v, want acp/0f8fad5b at the pushed commit %s", got.Pushed, pushed)
	}
}
