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
// checks out the session branch.
type Provider struct {
	root string
}

func NewProvider(root string) *Provider {
	return &Provider{root: root}
}

func (p *Provider) Prepare(ctx context.Context, agent domain.Agent, id domain.SessionID) (domain.Workspace, error) {
	dir := filepath.Join(p.root, string(id), "workspace")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return domain.Workspace{}, err
	}
	src := agent.Target.Path
	dest := filepath.Join(dir, filepath.Base(src))
	if _, err := git(ctx, "", "clone", "--quiet", "--", src, dest); err != nil {
		return domain.Workspace{}, err
	}
	if upstream, err := git(ctx, src, "remote", "get-url", "origin"); err == nil {
		if _, err := git(ctx, dest, "remote", "set-url", "origin", upstream); err != nil {
			return domain.Workspace{}, err
		}
	}
	if _, err := git(ctx, dest, "switch", "--quiet", "-c", id.Branch()); err != nil {
		return domain.Workspace{}, err
	}
	return domain.Workspace{Path: dest}, nil
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
