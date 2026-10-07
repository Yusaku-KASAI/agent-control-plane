package gitworkspace_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
