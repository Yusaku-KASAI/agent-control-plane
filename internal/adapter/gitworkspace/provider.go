package gitworkspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// Provider clones the target repository into <root>/<session id>/workspace/<repo name>, points
// origin at the target's own origin (the target path is not reachable from the container), and
// checks out the session branch: from the remote default branch when credentials allow fetching
// it, otherwise from the target's current HEAD.
type Provider struct {
	root string
}

func NewProvider(root string) *Provider {
	return &Provider{root: root}
}

func (p *Provider) Prepare(ctx context.Context, agent domain.Agent, id domain.SessionID, creds domain.Credentials) (domain.Workspace, error) {
	dir := filepath.Join(p.root, string(id), "workspace")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return domain.Workspace{}, err
	}
	src := agent.Target.Path
	dest := filepath.Join(dir, filepath.Base(src))
	g := gitRunner{token: creds.GitHubToken}

	if _, err := g.run(ctx, "", "clone", "--quiet", "--", src, dest); err != nil {
		return domain.Workspace{}, err
	}
	upstream, err := g.run(ctx, src, "remote", "get-url", "origin")
	hasUpstream := err == nil
	if hasUpstream {
		if _, err := g.run(ctx, dest, "remote", "set-url", "origin", upstream); err != nil {
			return domain.Workspace{}, err
		}
	}

	start := "HEAD"
	if hasUpstream && g.token != "" {
		if _, err := g.run(ctx, dest, "fetch", "--quiet", "--prune", "origin"); err != nil {
			return domain.Workspace{}, err
		}
		if _, err := g.run(ctx, dest, "remote", "set-head", "origin", "--auto"); err != nil {
			return domain.Workspace{}, err
		}
		start = "refs/remotes/origin/HEAD"
	}
	if _, err := g.run(ctx, dest, "switch", "--quiet", "--no-track", "-c", id.Branch(), start); err != nil {
		return domain.Workspace{}, err
	}
	return domain.Workspace{Path: dest}, nil
}

// gitRunner ignores the host user's git configuration, credential helpers and SSH keys, so the
// result does not depend on whose machine or container runs the control plane. Bind-mounted
// repositories may show a foreign owner inside a container, hence safe.directory.
type gitRunner struct {
	token string
}

const tokenHelper = `!f() { test "$1" = get && echo username=x-access-token && echo "password=$ACP_GIT_TOKEN"; }; f`

func (g gitRunner) run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"ACP_GIT_TOKEN="+g.token,
		"GIT_CONFIG_COUNT=4",
		"GIT_CONFIG_KEY_0=credential.helper", "GIT_CONFIG_VALUE_0="+tokenHelper,
		"GIT_CONFIG_KEY_1=url.https://github.com/.insteadOf", "GIT_CONFIG_VALUE_1=git@github.com:",
		"GIT_CONFIG_KEY_2=url.https://github.com/.insteadOf", "GIT_CONFIG_VALUE_2=ssh://git@github.com/",
		"GIT_CONFIG_KEY_3=safe.directory", "GIT_CONFIG_VALUE_3=*",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
