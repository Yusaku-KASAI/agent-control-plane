package docker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

func TestSessionFiles_Remove(t *testing.T) {
	const other domain.SessionID = "11111111-1111-4111-8111-111111111111"
	id := domain.SessionID("0f8fad5b-d9cb-469f-a165-70867728950e")
	root := t.TempDir()
	opts := Options{ClaudeConfigDir: filepath.Join(root, "claude"), CredentialsDir: filepath.Join(root, "credentials")}
	agent := filepath.Join(opts.ClaudeConfigDir, "repo-dev")
	removed := []string{
		filepath.Join(opts.CredentialsDir, string(id), "gh", "hosts.yml"),
		filepath.Join(agent, "projects", "-workspace-repo", string(id)+".jsonl"),
		filepath.Join(agent, "projects", "-workspace-repo", string(id), "subagents", "a.jsonl"),
		filepath.Join(agent, "session-env", string(id), "env"),
		filepath.Join(agent, "todos", string(id)+"-agent-"+string(id)+".json"),
	}
	kept := []string{
		filepath.Join(opts.CredentialsDir, string(other), "gh", "hosts.yml"),
		filepath.Join(agent, "projects", "-workspace-repo", string(other)+".jsonl"),
		filepath.Join(agent, "settings.json"),
		filepath.Join(opts.ClaudeConfigDir, "repo-guide", "projects", "-workspace-repo", string(id)+".jsonl"),
	}
	for _, f := range append(removed, kept...) {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := NewSessionFiles(opts).Remove(context.Background(), "repo-dev", id); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	for _, f := range removed {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("%s still exists", f)
		}
	}
	for _, f := range kept {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}
