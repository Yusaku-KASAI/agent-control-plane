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

// Provider clones the target repository into <root>/<session id>/workspace/<repo name>.
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
	dest := filepath.Join(dir, filepath.Base(agent.Target.Path))
	out, err := exec.CommandContext(ctx, "git", "clone", "--quiet", "--", agent.Target.Path, dest).CombinedOutput()
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("git clone %s: %w: %s", agent.Target.Path, err, strings.TrimSpace(string(out)))
	}
	return domain.Workspace{Path: dest}, nil
}
