package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
