package gitworkspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// The hooks are shell kept in Go strings, out of reach of the shellcheck that make check runs on
// the scripts in files. This has installHooks write them and checks each one. It is an internal
// test because installHooks is unexported. shellcheck is in the devcontainer image that the checks
// run in, so a missing one fails rather than skips.
func TestInstalledHooksPassShellcheck(t *testing.T) {
	shellcheck, err := exec.LookPath("shellcheck")
	if err != nil {
		t.Fatalf("shellcheck is not installed: %v", err)
	}
	repo := t.TempDir()
	if err := installHooks(repo, domain.SessionID("0f8fad5b-d9cb-469f-a165-70867728950e")); err != nil {
		t.Fatalf("installHooks() error = %v", err)
	}
	dir := filepath.Join(repo, ".git", "hooks")
	hooks, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks) == 0 {
		t.Fatal("installHooks wrote no hooks")
	}
	for _, h := range hooks {
		out, err := exec.Command(shellcheck, filepath.Join(dir, h.Name())).CombinedOutput()
		if err != nil {
			t.Errorf("shellcheck %s: %v\n%s", h.Name(), err, out)
		}
	}
}
