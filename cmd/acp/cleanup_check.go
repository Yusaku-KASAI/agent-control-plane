package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/githubapp"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/gitworkspace"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// cleanupCheck prints the reasons the cleanup would keep the session whose workspace holds the working
// directory, one per line as the kind and the text, or that there are none. The agent runs it in its
// container (the harness contract), where the session's branch and GitHub token are given.
func cleanupCheck(ctx context.Context, getenv func(string) string, stdout, stderr io.Writer) int {
	branch := getenv("ACP_SESSION_BRANCH")
	if branch == "" {
		fmt.Fprintln(stderr, "acp: ACP_SESSION_BRANCH is not set; cleanup-check runs in an agent's container")
		return 2
	}
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, "acp:", err)
		return 1
	}
	// The cleanup looks at the whole workspace, wherever in it this runs.
	dir, err := gitworkspace.Root(ctx, wd)
	if err != nil {
		fmt.Fprintln(stderr, "acp: not in the session's workspace:", err)
		return 1
	}
	token, err := githubToken(getenv("GH_CONFIG_DIR"))
	if err != nil {
		fmt.Fprintln(stderr, "acp:", err)
		return 1
	}
	// The cleanup asks the target's remote, which the workspace's origin is (the harness contract).
	origin := func(ctx context.Context, _ string) (string, error) { return gitworkspace.Origin(ctx, dir) }
	branches := githubapp.NewBranches(githubapp.DefaultBaseURL, &http.Client{Timeout: 30 * time.Second}, origin)
	reasons, err := usecase.CheckCleanup(ctx, ownWorkspace{gitworkspace.NewProvider("")}, branches, domain.Agent{}, branch, dir,
		domain.Credentials{GitHubToken: token})
	if err != nil {
		fmt.Fprintln(stderr, "acp:", err)
		return 1
	}
	if len(reasons) == 0 {
		fmt.Fprintln(stdout, "片付けを止めるものはありません。")
		return 0
	}
	for _, r := range reasons {
		fmt.Fprintf(stdout, "%s\t%s\n", r.Kind, r.Text)
	}
	return 0
}

// ownWorkspace reads the workspace in place: in the agent's own container, what the agent wrote
// runs only there.
type ownWorkspace struct {
	provider *gitworkspace.Provider
}

func (w ownWorkspace) State(ctx context.Context, _ domain.Agent, path string) (usecase.WorkspaceState, error) {
	return w.provider.State(ctx, path)
}

// githubToken reads the token the control plane writes into gh's config, or "" without a GitHub
// App (no config). With a config but no token, it fails: judging without the remote would miss
// what the cleanup sees there, such as open pull requests.
func githubToken(configDir string) (string, error) {
	if configDir == "" {
		return "", nil
	}
	data, err := os.ReadFile(filepath.Join(configDir, "hosts.yml"))
	if err != nil {
		return "", fmt.Errorf("read the GitHub token: %w", err)
	}
	var hosts map[string]struct {
		OAuthToken string `yaml:"oauth_token"`
	}
	if err := yaml.Unmarshal(data, &hosts); err != nil {
		return "", fmt.Errorf("read gh hosts: %w", err)
	}
	if hosts["github.com"].OAuthToken == "" {
		return "", errors.New("read the GitHub token: none for github.com in gh's hosts")
	}
	return hosts["github.com"].OAuthToken, nil
}
