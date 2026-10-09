package gitworkspace_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/gitworkspace"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
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

func TestProvider_Branches(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	p := gitworkspace.NewProvider(t.TempDir())
	agent := domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: f.source, Image: "img"}}
	ws, err := p.Prepare(ctx, agent, sessionID, domain.Credentials{GitHubToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	type branch struct {
		name, upstream string
		unpushed       bool
	}
	check := func(want ...branch) {
		t.Helper()
		got, err := p.Branches(ctx, ws.Path)
		if err != nil {
			t.Fatal(err)
		}
		var names []branch
		for _, b := range got {
			if b.Name != "HEAD" && b.Tip != gitOutput(t, ws.Path, "rev-parse", "refs/heads/"+b.Name) {
				t.Errorf("tip of %s = %q", b.Name, b.Tip)
			}
			names = append(names, branch{b.Name, b.Upstream, b.Unpushed})
		}
		if !slices.Equal(names, want) {
			t.Errorf("Branches() = %+v, want %+v", names, want)
		}
	}

	// The session branch tracks origin before it is pushed; the clone's branch, feature, is gone.
	check(branch{"acp/0f8fad5b", "acp/0f8fad5b", false})

	commit(t, ws.Path, "a.txt", "session work")
	check(branch{"acp/0f8fad5b", "acp/0f8fad5b", true})
	git(t, ws.Path, "push", "--quiet", "origin", "acp/0f8fad5b")
	check(branch{"acp/0f8fad5b", "acp/0f8fad5b", false})

	git(t, ws.Path, "switch", "--quiet", "-c", "acp/0f8fad5b-docs")
	commit(t, ws.Path, "b.txt", "docs")
	git(t, ws.Path, "push", "--quiet", "-u", "origin", "HEAD")
	git(t, ws.Path, "switch", "--quiet", "-c", "scratch")
	commit(t, ws.Path, "c.txt", "scratch")
	git(t, ws.Path, "tag", "scratch")
	git(t, ws.Path, "switch", "--quiet", "--detach", "acp/0f8fad5b")
	commit(t, ws.Path, "d.txt", "detached")
	check(
		branch{"acp/0f8fad5b", "acp/0f8fad5b", false},
		branch{"acp/0f8fad5b-docs", "acp/0f8fad5b-docs", false},
		branch{"scratch", "", true},
		branch{"HEAD", "", true},
	)
}

func TestProvider_Branches_ClonedBranchOfOlderWorkspace(t *testing.T) {
	f := newFixture(t)
	ws := filepath.Join(t.TempDir(), "repo")
	git(t, "", "clone", "--quiet", f.source, ws)
	git(t, ws, "switch", "--quiet", "-c", "acp/0f8fad5b")
	got, err := gitworkspace.NewProvider(t.TempDir()).Branches(context.Background(), ws)
	if err != nil || len(got) != 1 || got[0].Name != "acp/0f8fad5b" {
		t.Errorf("Branches() = %+v, %v; want only the session branch", got, err)
	}
}

func TestProvider_Branches_NoRepository(t *testing.T) {
	p := gitworkspace.NewProvider(t.TempDir())
	// A directory inside another repository must not be read as that repository.
	inside := filepath.Join(newFixture(t).source, "sub")
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", filepath.Join(t.TempDir(), "gone"), inside} {
		if got, err := p.Branches(context.Background(), path); got != nil || err != nil {
			t.Errorf("Branches(%q) = %v, %v; want nothing", path, got, err)
		}
	}
}

func TestProvider_Prepare_WithoutCredentialsLeavesSessionBranchUntracked(t *testing.T) {
	f := newFixture(t)
	agent := domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: f.source, Image: "img"}}
	p := gitworkspace.NewProvider(t.TempDir())
	ws, err := p.Prepare(context.Background(), agent, sessionID, domain.Credentials{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Branches(context.Background(), ws.Path)
	if err != nil || len(got) != 1 || got[0].Upstream != "" || got[0].Unpushed {
		t.Errorf("Branches() = %+v, %v; want the session branch, untracked and with nothing unpushed", got, err)
	}
}
