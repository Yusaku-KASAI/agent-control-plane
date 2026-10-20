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

func TestProvider_Prepare_ChecksBeforePush(t *testing.T) {
	// The target's check passes when ACP_TEST_CHECK is "pass", fails when git variables leak into
	// it, and records that it ran.
	const makefile = "check:\n\ttest \"$$ACP_TEST_CHECK\" = pass\n\ttest -z \"$$GIT_DIR$$GIT_INDEX_FILE\"\n\ttouch \"$$ACP_TEST_CHECKED\"\n"
	tests := []struct {
		name     string
		makefile bool
		check    string
		// prepare returns the directory to push from.
		prepare  func(t *testing.T, ws string) string
		refspec  string
		wantPush bool
		wantRan  bool
	}{
		{name: "the check passes", makefile: true, check: "pass", refspec: "HEAD:refs/heads/topic", wantPush: true, wantRan: true},
		{name: "the check fails", makefile: true, check: "fail", refspec: "HEAD:refs/heads/topic"},
		{
			name: "an untracked file", makefile: true, check: "pass", refspec: "HEAD:refs/heads/topic",
			prepare: func(t *testing.T, ws string) string {
				if err := os.WriteFile(filepath.Join(ws, "stray"), nil, 0o644); err != nil {
					t.Fatal(err)
				}
				return ws
			},
		},
		{
			name: "a change to a tracked file", makefile: true, check: "pass", refspec: "HEAD:refs/heads/topic",
			prepare: func(t *testing.T, ws string) string {
				if err := os.WriteFile(filepath.Join(ws, "README.md"), []byte("edited"), 0o644); err != nil {
					t.Fatal(err)
				}
				return ws
			},
		},
		{
			name: "an ignored file", makefile: true, check: "pass", refspec: "HEAD:refs/heads/topic", wantPush: true, wantRan: true,
			prepare: func(t *testing.T, ws string) string {
				commit(t, ws, ".gitignore", "build/\n")
				if err := os.MkdirAll(filepath.Join(ws, "build"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(ws, "build", "out"), nil, 0o644); err != nil {
					t.Fatal(err)
				}
				return ws
			},
		},
		{
			name: "an annotated tag on HEAD", makefile: true, check: "pass", refspec: "refs/tags/v1", wantPush: true, wantRan: true,
			prepare: func(t *testing.T, ws string) string {
				git(t, ws, "-c", "user.name=test", "-c", "user.email=test@example.com", "tag", "-a", "-m", "v1", "v1")
				return ws
			},
		},
		{
			name: "a commit other than HEAD", makefile: true, check: "pass", refspec: "HEAD~1:refs/heads/topic",
		},
		{name: "a target without a Makefile", check: "fail", refspec: "HEAD:refs/heads/topic", wantPush: true},
		{
			name:  "a target without a Makefile, pushing a commit other than HEAD from a work tree with a stray file",
			check: "fail", refspec: "HEAD~1:refs/heads/topic", wantPush: true,
			prepare: func(t *testing.T, ws string) string {
				if err := os.WriteFile(filepath.Join(ws, "stray"), nil, 0o644); err != nil {
					t.Fatal(err)
				}
				return ws
			},
		},
		{
			name: "deleting a branch", makefile: true, check: "fail", refspec: ":refs/heads/old", wantPush: true,
			prepare: func(t *testing.T, ws string) string {
				git(t, ws, "push", "--quiet", "--no-verify", "origin", "HEAD:refs/heads/old")
				return ws
			},
		},
		{
			name: "from a linked worktree", makefile: true, check: "pass", refspec: "HEAD:refs/heads/topic", wantPush: true, wantRan: true,
			prepare: func(t *testing.T, ws string) string {
				wt := filepath.Join(t.TempDir(), "wt")
				git(t, ws, "worktree", "add", "--quiet", "-b", "linked", wt)
				return wt
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			agent := domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: f.source, Image: "img"}}
			ws, err := gitworkspace.NewProvider(t.TempDir()).Prepare(context.Background(), agent, sessionID, domain.Credentials{})
			if err != nil {
				t.Fatalf("Prepare() error = %v", err)
			}
			if tt.makefile {
				commit(t, ws.Path, "Makefile", makefile)
			} else {
				commit(t, ws.Path, "change.txt", "agent work")
			}
			dir := ws.Path
			if tt.prepare != nil {
				dir = tt.prepare(t, ws.Path)
			}
			checked := filepath.Join(t.TempDir(), "checked")
			t.Setenv("ACP_TEST_CHECK", tt.check)
			t.Setenv("ACP_TEST_CHECKED", checked)

			before := gitOutput(t, f.remote, "for-each-ref")
			cmd := exec.Command("git", "push", "--quiet", "origin", tt.refspec)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if pushed := err == nil; pushed != tt.wantPush {
				t.Fatalf("push succeeded = %v, want %v: %s", pushed, tt.wantPush, out)
			}
			if !tt.wantPush && gitOutput(t, f.remote, "for-each-ref") != before {
				t.Errorf("a refused push changed the remote")
			}
			if _, err := os.Stat(checked); (err == nil) != tt.wantRan {
				t.Errorf("check ran = %v, want %v", err == nil, tt.wantRan)
			}
		})
	}
}

func TestProvider_Prepare_WarnsOfNewBranchNames(t *testing.T) {
	const warning = "acp pre-push: 注意"
	// The hook copies the naming rule of SessionID.OwnsBranch into shell, so a new branch is
	// expected to warn exactly when the session does not own its name.
	newBranch := func(name string) string { return "HEAD:refs/heads/" + name }
	tests := []struct {
		name     string
		makefile bool
		// prepare runs before the push, from the workspace.
		prepare func(t *testing.T, ws string)
		refspec string
		// branch is the name of the new remote branch the push makes, if any.
		branch string
		// also is another refspec pushed with it.
		also     string
		wantFail bool
	}{
		{name: "the session branch", branch: "acp/0f8fad5b"},
		{name: "a branch the session splits off", branch: "acp/0f8fad5b-topic"},
		{name: "another name", branch: "topic"},
		{name: "another name in a target with a Makefile", makefile: true, branch: "topic"},
		{name: "a name the session branch only starts", branch: "acp/0f8fad5bx"},
		{name: "another session's branch", branch: "acp/1234abcd"},
		{
			name: "a branch that is already on the remote", refspec: newBranch("topic"),
			prepare: func(t *testing.T, ws string) {
				git(t, ws, "push", "--quiet", "--no-verify", "origin", "HEAD~1:refs/heads/topic")
			},
		},
		{
			name: "deleting a branch", refspec: ":refs/heads/old",
			prepare: func(t *testing.T, ws string) {
				git(t, ws, "push", "--quiet", "--no-verify", "origin", "HEAD:refs/heads/old")
			},
		},
		{name: "deleting a branch that is not on the remote", refspec: ":refs/heads/gone"},
		{name: "a tag", refspec: "HEAD:refs/tags/v1"},
		{name: "another name after the session branch in one push", also: "HEAD:refs/heads/acp/0f8fad5b", branch: "topic"},
		{
			name: "another name in a push the checks refuse", makefile: true, branch: "topic", wantFail: true,
			prepare: func(t *testing.T, ws string) {
				if err := os.WriteFile(filepath.Join(ws, "stray"), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			agent := domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: f.source, Image: "img"}}
			ws, err := gitworkspace.NewProvider(t.TempDir()).Prepare(context.Background(), agent, sessionID, domain.Credentials{})
			if err != nil {
				t.Fatalf("Prepare() error = %v", err)
			}
			if tt.makefile {
				commit(t, ws.Path, "Makefile", "check:\n\ttrue\n")
			} else {
				commit(t, ws.Path, "change.txt", "agent work")
			}
			if tt.prepare != nil {
				tt.prepare(t, ws.Path)
			}
			refspec, wantWarn := tt.refspec, false
			if tt.branch != "" {
				refspec, wantWarn = newBranch(tt.branch), !sessionID.OwnsBranch(tt.branch)
			}

			args := []string{"push", "--quiet", "origin"}
			if tt.also != "" {
				args = append(args, tt.also)
			}
			cmd := exec.Command("git", append(args, refspec)...)
			cmd.Dir = ws.Path
			out, err := cmd.CombinedOutput()
			if failed := err != nil; failed != tt.wantFail {
				t.Fatalf("push failed = %v, want %v: %s", failed, tt.wantFail, out)
			}
			if warned := strings.Contains(string(out), warning); warned != wantWarn {
				t.Errorf("warned = %v, want %v: %s", warned, wantWarn, out)
			}
		})
	}
}

func TestProvider_Prepare_RefusesToOverwriteWhatWasNeverTaken(t *testing.T) {
	const branch = "acp/0f8fad5b"
	amend := func(t *testing.T, ws string) {
		git(t, ws, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "--quiet", "--amend", "-m", "rewritten")
	}
	tests := []struct {
		name string
		// other pushes to the branch from another clone before the workspace pushes, when set.
		other bool
		// prepare runs in the workspace after that, before the push.
		prepare func(t *testing.T, ws string)
		// args are those of git push; <head> stands for the hash of the workspace's HEAD.
		args     []string
		wantPush bool
	}{
		{
			name: "a fast-forward after taking the other's commit", other: true, wantPush: true,
			prepare: func(t *testing.T, ws string) {
				git(t, ws, "pull", "--quiet", "--rebase", "origin", branch)
				commit(t, ws, "more.txt", "more")
			},
			args: []string{"origin", "HEAD:refs/heads/" + branch},
		},
		{
			name: "rewriting what only this workspace pushed", wantPush: true,
			prepare: amend,
			args:    []string{"--force", "origin", "HEAD:refs/heads/" + branch},
		},
		{
			name: "rewriting over a commit never fetched", other: true,
			prepare: amend,
			args:    []string{"--force", "origin", "HEAD:refs/heads/" + branch},
		},
		{
			name: "rewriting over a commit fetched but not taken", other: true,
			prepare: func(t *testing.T, ws string) {
				amend(t, ws)
				git(t, ws, "fetch", "--quiet", "origin")
			},
			args: []string{"--force", "origin", "HEAD:refs/heads/" + branch},
		},
		{
			name: "rewriting after only looking at the other's commit", other: true,
			prepare: func(t *testing.T, ws string) {
				git(t, ws, "fetch", "--quiet", "origin")
				git(t, ws, "switch", "--quiet", "--detach", "origin/"+branch)
				git(t, ws, "switch", "--quiet", branch)
				amend(t, ws)
			},
			args: []string{"--force", "origin", "HEAD:refs/heads/" + branch},
		},
		{
			name: "rewriting after taking the other's commit", other: true, wantPush: true,
			prepare: func(t *testing.T, ws string) {
				git(t, ws, "pull", "--quiet", "--rebase", "origin", branch)
				commit(t, ws, "mine.txt", "mine")
				amend(t, ws)
			},
			args: []string{"--force", "origin", "HEAD:refs/heads/" + branch},
		},
		{
			name: "a fast-forward of a commit named by its hash, which has no reflog", wantPush: true,
			prepare: func(t *testing.T, ws string) {
				commit(t, ws, "more.txt", "more")
			},
			args: []string{"origin", "<head>:refs/heads/" + branch},
		},
		{
			name: "deleting the branch", other: true, wantPush: true,
			args: []string{"origin", ":refs/heads/" + branch},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			agent := domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: f.source, Image: "img"}}
			ws, err := gitworkspace.NewProvider(t.TempDir()).Prepare(context.Background(), agent, sessionID, domain.Credentials{})
			if err != nil {
				t.Fatalf("Prepare() error = %v", err)
			}
			commit(t, ws.Path, "change.txt", "agent work")
			git(t, ws.Path, "push", "--quiet", "origin", "HEAD:refs/heads/"+branch)
			if tt.other {
				other := filepath.Join(t.TempDir(), "other")
				git(t, "", "clone", "--quiet", "--branch", branch, f.remote, other)
				commit(t, other, "other.txt", "the other's work")
				git(t, other, "push", "--quiet", "origin", branch)
			}
			if tt.prepare != nil {
				tt.prepare(t, ws.Path)
			}

			before := gitOutput(t, f.remote, "for-each-ref")
			args := []string{"push", "--quiet"}
			for _, a := range tt.args {
				args = append(args, strings.ReplaceAll(a, "<head>", gitOutput(t, ws.Path, "rev-parse", "HEAD")))
			}
			cmd := exec.Command("git", args...)
			cmd.Dir = ws.Path
			out, err := cmd.CombinedOutput()
			if pushed := err == nil; pushed != tt.wantPush {
				t.Fatalf("push succeeded = %v, want %v: %s", pushed, tt.wantPush, out)
			}
			if !tt.wantPush && gitOutput(t, f.remote, "for-each-ref") != before {
				t.Errorf("a refused push changed the remote")
			}
		})
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
