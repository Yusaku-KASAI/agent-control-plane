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
	env := map[string]string{"ACP_SESSION_BRANCH": "acp/0f8fad5b", "ACP_WORKSPACE": dir}
	run := func() (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := dispatch(context.Background(), []string{"cleanup-check"}, func(k string) string { return env[k] }, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}

	t.Chdir(dir)
	if code, out, errOut := run(); code != 0 || !strings.Contains(out, "ありません") {
		t.Errorf("cleanup-check in a tidy workspace = %d, %q (stderr %q); want none", code, out, errOut)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("untracked"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := run(); code != 0 || !strings.HasPrefix(out, "changes\t") {
		t.Errorf("cleanup-check with an untracked file = %d, %q (stderr %q); want the changes", code, out, errOut)
	}

	// From a tidy repository outside the workspace, such as one cloned into /tmp, the workspace is
	// checked all the same.
	outside := t.TempDir()
	if out, err := exec.Command("git", "init", "--quiet", outside).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	t.Chdir(outside)
	if code, out, errOut := run(); code != 0 || !strings.HasPrefix(out, "changes\t") {
		t.Errorf("cleanup-check from a repository outside = %d, %q (stderr %q); want the workspace's changes", code, out, errOut)
	}

	for _, unset := range []string{"ACP_SESSION_BRANCH", "ACP_WORKSPACE"} {
		v := env[unset]
		delete(env, unset)
		if code, _, _ := run(); code != 2 {
			t.Errorf("cleanup-check without %s = %d, want 2", unset, code)
		}
		env[unset] = v
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
