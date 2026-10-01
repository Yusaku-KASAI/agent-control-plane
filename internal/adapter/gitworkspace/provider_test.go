package gitworkspace_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/gitworkspace"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

func TestProvider_Prepare(t *testing.T) {
	source := filepath.Join(t.TempDir(), "repo")
	git(t, "", "init", "--quiet", source)
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("committed"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, source, "add", "README.md")
	git(t, source, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "--quiet", "-m", "init")
	if err := os.WriteFile(filepath.Join(source, "dirty.txt"), []byte("uncommitted"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	agent := domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: source, Image: "img"}}
	p := gitworkspace.NewProvider(root)

	first, err := p.Prepare(context.Background(), agent, "0f8fad5b-d9cb-469f-a165-70867728950e")
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if want := filepath.Join(root, "0f8fad5b-d9cb-469f-a165-70867728950e", "workspace", "repo"); first.Path != want {
		t.Errorf("Path = %q, want %q", first.Path, want)
	}
	if b, err := os.ReadFile(filepath.Join(first.Path, "README.md")); err != nil || string(b) != "committed" {
		t.Errorf("README.md = %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(first.Path, "dirty.txt")); !os.IsNotExist(err) {
		t.Errorf("uncommitted file was copied (stat err = %v)", err)
	}
}

func TestProvider_Prepare_NotARepository(t *testing.T) {
	agent := domain.Agent{Name: "a", Role: "developer", Target: domain.Target{Path: t.TempDir(), Image: "img"}}
	if _, err := gitworkspace.NewProvider(t.TempDir()).Prepare(context.Background(), agent, "0f8fad5b-d9cb-469f-a165-70867728950e"); err == nil {
		t.Fatal("Prepare() error = nil, want error")
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
