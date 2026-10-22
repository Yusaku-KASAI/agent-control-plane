package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

func TestDispatch_CleanupCheck(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "--quiet", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	t.Chdir(dir)
	env := map[string]string{"ACP_SESSION_BRANCH": "acp/0f8fad5b"}
	run := func() (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := dispatch(context.Background(), []string{"cleanup-check"}, func(k string) string { return env[k] }, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}

	if code, out, errOut := run(); code != 0 || !strings.Contains(out, "ありません") {
		t.Errorf("cleanup-check in a tidy workspace = %d, %q (stderr %q); want none", code, out, errOut)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("untracked"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := run(); code != 0 || !strings.HasPrefix(out, "changes\t") {
		t.Errorf("cleanup-check with an untracked file = %d, %q (stderr %q); want the changes", code, out, errOut)
	}

	// From a subdirectory or a linked worktree, the whole workspace is checked.
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	if code, out, errOut := run(); code != 0 || !strings.HasPrefix(out, "changes\t") {
		t.Errorf("cleanup-check from a subdirectory = %d, %q (stderr %q); want the changes", code, out, errOut)
	}
	for _, args := range [][]string{
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "--quiet", "--allow-empty", "-m", "first"},
		{"worktree", "add", "--quiet", filepath.Join(dir, ".claude", "worktrees", "a")},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	t.Chdir(filepath.Join(dir, ".claude", "worktrees", "a"))
	if code, out, errOut := run(); code != 0 || !strings.Contains(out, "changes\t") {
		t.Errorf("cleanup-check from a linked worktree = %d, %q (stderr %q); want the main worktree's changes", code, out, errOut)
	}
	// From a submodule, the workspace that holds it.
	module := filepath.Join(t.TempDir(), "module")
	for _, args := range [][]string{
		{"init", "--quiet", module},
		{"-C", module, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "--quiet", "--allow-empty", "-m", "m"},
		{"-C", dir, "-c", "protocol.file.allow=always", "submodule", "--quiet", "add", module, "mod"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	t.Chdir(filepath.Join(dir, "mod"))
	if code, out, errOut := run(); code != 0 || !strings.Contains(out, "changes\t") {
		t.Errorf("cleanup-check from a submodule = %d, %q (stderr %q); want the workspace's changes", code, out, errOut)
	}
	t.Chdir(dir)

	delete(env, "ACP_SESSION_BRANCH")
	if code, _, _ := run(); code != 2 {
		t.Errorf("cleanup-check outside an agent's container = %d, want 2", code)
	}
}

// The token is where the control plane writes it for gh (internal/adapter/docker/credentials.go).
func TestGitHubToken(t *testing.T) {
	dir := t.TempDir()
	hosts := "github.com:\n    users:\n        x-access-token:\n            oauth_token: tok\n" +
		"    oauth_token: tok\n    user: x-access-token\n    git_protocol: https\n"
	if err := os.WriteFile(filepath.Join(dir, "hosts.yml"), []byte(hosts), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := githubToken(dir); err != nil || got != "tok" {
		t.Errorf("githubToken() = %q, %v; want tok", got, err)
	}
	if got, err := githubToken(t.TempDir()); err != nil || got != "" {
		t.Errorf("githubToken() without hosts = %q, %v; want none", got, err)
	}
}

// The kinds are published in the harness contract, where resolving each is looked up.
func TestCleanupCheck_KindsInHarnessContract(t *testing.T) {
	contract, err := os.ReadFile(filepath.Join("..", "..", "platform", "docs", "reference", "harness.md"))
	if err != nil {
		t.Fatal(err)
	}
	kinds := usecase.ReasonKinds()
	for _, kind := range kinds {
		if !strings.Contains(string(contract), "| `"+kind+"` |") {
			t.Errorf("kind %s is not in the harness contract's table", kind)
		}
	}
	for _, m := range regexp.MustCompile("(?m)^  \\| `([a-z-]+)` \\|").FindAllStringSubmatch(string(contract), -1) {
		if !slices.Contains(kinds, m[1]) {
			t.Errorf("the harness contract tells kind %s, which cleanup-check does not", m[1])
		}
	}
}
