package gitworkspace

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// Whoever checks what the reading says knows the operations by usecase.Operations.
func TestOperations_AreKnown(t *testing.T) {
	for _, op := range operations {
		if !slices.Contains(usecase.Operations, op.name) {
			t.Errorf("operation %q is not in usecase.Operations", op.name)
		}
	}
}

// git asks the credential helper of gitRunner for each remote it reaches; only github.com may get
// the token, whatever the agent set the workspace's remotes to.
func TestGitRunner_GivesTheTokenToGitHubAlone(t *testing.T) {
	g := gitRunner{token: "ghs_secret"}
	for url, want := range map[string]bool{
		"https://github.com/owner/repo.git":         true,
		"https://x-access-token@github.com/o/r.git": true,
		"http://github.com/owner/repo.git":          false,
		"https://github.com.evil.example/o/r.git":   false,
		"https://evil.example/owner/repo.git":       false,
		"https://gitlab.com/owner/repo.git":         false,
		"https://api.github.com/owner/repo.git":     false,
	} {
		cmd := exec.Command("git", "credential", "fill")
		cmd.Dir = t.TempDir()
		cmd.Env = g.env()
		cmd.Stdin = strings.NewReader("url=" + url + "\n\n")
		out, _ := cmd.CombinedOutput()
		if got := strings.Contains(string(out), "password=ghs_secret"); got != want {
			t.Errorf("git credential fill for %s gives the token = %v, want %v (output %q)", url, got, want, out)
		}
	}
}
