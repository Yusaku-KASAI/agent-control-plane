package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// What the subcommand prints is what the control plane decodes as the workspace state.
func TestDispatch_WorkspaceState(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "--quiet", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("untracked"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	code := dispatch(context.Background(), []string{"workspace-state"}, func(string) string { return "" }, &stdout, &stderr)

	var got usecase.WorkspaceState
	if err := json.Unmarshal(stdout.Bytes(), &got); code != 0 || err != nil || !got.Changes {
		t.Errorf("workspace-state = %d, %+v, %v (stderr: %s); want the untracked file as changes", code, got, err, stderr.String())
	}
}
