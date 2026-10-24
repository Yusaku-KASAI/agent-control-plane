package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// The token read in the agent's container is the one written for gh.
func TestReadGitHubToken(t *testing.T) {
	c := sessionCredentials{root: t.TempDir(), id: domain.SessionID("0f8fad5b-d9cb-469f-a165-70867728950e")}
	if err := c.write(domain.Credentials{GitHubToken: "ghs_tok"}); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadGitHubToken(filepath.Join(c.dir(), "gh")); err != nil || got != "ghs_tok" {
		t.Errorf("ReadGitHubToken() = %q, %v; want ghs_tok", got, err)
	}
	if got, err := ReadGitHubToken(""); err != nil || got != "" {
		t.Errorf("ReadGitHubToken() without a GitHub App = %q, %v; want none", got, err)
	}
	// With a GitHub App, judging without the token would miss what the cleanup sees on the remote.
	if got, err := ReadGitHubToken(t.TempDir()); err == nil {
		t.Errorf("ReadGitHubToken() with no hosts = %q; want an error", got)
	}
	empty := t.TempDir()
	if err := os.WriteFile(filepath.Join(empty, "hosts.yml"), []byte("github.com:\n    user: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadGitHubToken(empty); err == nil {
		t.Errorf("ReadGitHubToken() with no token in hosts = %q; want an error", got)
	}
}
