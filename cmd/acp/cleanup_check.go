package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/docker"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/githubapp"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/gitworkspace"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// cleanupCheck prints the reasons the cleanup would keep the session, one per line as the kind and
// the text, or that there are none. The agent runs it in its container (the harness contract), where
// the session's branch, workspace and GitHub token are given.
func cleanupCheck(ctx context.Context, getenv func(string) string, stdout, stderr io.Writer) int {
	branch, dir := getenv("ACP_SESSION_BRANCH"), getenv("ACP_WORKSPACE")
	for _, v := range [][2]string{{"ACP_SESSION_BRANCH", branch}, {"ACP_WORKSPACE", dir}} {
		if v[1] == "" {
			fmt.Fprintf(stderr, "acp: %s is not set; cleanup-check runs in an agent's container the control plane started"+
				" (one started by an older control plane may lack it)\n", v[0])
			return 2
		}
	}
	token, err := docker.ReadGitHubToken(getenv("GH_CONFIG_DIR"))
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
