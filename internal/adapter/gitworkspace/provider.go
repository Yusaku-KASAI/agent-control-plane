package gitworkspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
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

	cloned, err := g.run(ctx, dest, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		cloned = ""
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
	// The clone's branch is the target's checkout, not the session's work.
	if cloned != "" {
		if _, err := g.run(ctx, dest, "branch", "--quiet", "-D", cloned); err != nil {
			return domain.Workspace{}, err
		}
	}
	if start != "HEAD" {
		// The session branch tracks its namesake on origin before it is pushed, so that it counts as
		// the session's however the agent pushes it.
		for key, value := range map[string]string{"remote": "origin", "merge": "refs/heads/" + id.Branch()} {
			if _, err := g.run(ctx, dest, "config", "branch."+id.Branch()+"."+key, value); err != nil {
				return domain.Workspace{}, err
			}
		}
	}
	if err := installCommitMsgHook(dest); err != nil {
		return domain.Workspace{}, err
	}
	return domain.Workspace{Path: dest}, nil
}

// commitMsgHook adds a Co-authored-by trailer for each person in $ACP_CO_AUTHORS_FILE. It lives
// only in the workspace's .git, so it is never committed; a target that sets core.hooksPath
// bypasses it.
const commitMsgHook = `#!/bin/sh
f="${ACP_CO_AUTHORS_FILE:-}"
[ -n "$f" ] && [ -r "$f" ] || exit 0
while IFS= read -r who; do
	[ -n "$who" ] || continue
	git interpret-trailers --in-place --if-exists addIfDifferent --trailer "Co-authored-by: $who" "$1" || exit 1
done <"$f"
`

func installCommitMsgHook(repo string) error {
	dir := filepath.Join(repo, ".git", "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "commit-msg"), []byte(commitMsgHook), 0o755)
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

// Origin returns the origin URL of the repository at path, or "" when it has none.
func Origin(ctx context.Context, path string) (string, error) {
	g := gitRunner{}
	if _, err := g.run(ctx, path, "rev-parse", "--git-dir"); err != nil {
		return "", err
	}
	url, err := g.run(ctx, path, "remote", "get-url", "origin")
	if err != nil {
		return "", nil
	}
	return url, nil
}

// Branches tells the local branches by the tip each has and whether it has been pushed.
func (p *Provider) Branches(ctx context.Context, path string) ([]usecase.LocalBranch, error) {
	if ok, err := exists(filepath.Join(path, ".git")); !ok || err != nil {
		return nil, err
	}
	g := gitRunner{}
	out, err := g.run(ctx, path, "for-each-ref", "--format=%(refname:lstrip=2)%09%(objectname)%09%(upstream:remotename)%09%(upstream:remoteref)", "refs/heads")
	if err != nil {
		return nil, err
	}
	var branches []usecase.LocalBranch
	for line := range strings.Lines(out) {
		fields := strings.SplitN(strings.TrimRight(line, "\n"), "\t", 4)
		fields = append(fields, make([]string, 4-len(fields))...)
		b := usecase.LocalBranch{Name: fields[0], Tip: fields[1]}
		if upstream, ok := strings.CutPrefix(fields[3], "refs/heads/"); ok && fields[2] == "origin" {
			b.Upstream = upstream
		}
		created, err := createdByClone(ctx, g, path, b.Name)
		if err != nil {
			return nil, err
		}
		if created {
			continue
		}
		if b.Unpushed, err = hasUnpushed(ctx, g, path, "refs/heads/"+b.Name, "--remotes"); err != nil {
			return nil, err
		}
		branches = append(branches, b)
	}
	if _, err := g.run(ctx, path, "symbolic-ref", "--quiet", "HEAD"); err != nil {
		detached, err := hasUnpushed(ctx, g, path, "HEAD", "--branches", "--remotes")
		if err != nil {
			return nil, err
		}
		if detached {
			branches = append(branches, usecase.LocalBranch{Name: "HEAD", Unpushed: true})
		}
	}
	return branches, nil
}

// createdByClone tells the branch the clone created by its first reflog entry. Prepare deletes that
// branch, but workspaces prepared before it did still have it.
func createdByClone(ctx context.Context, g gitRunner, path, branch string) (bool, error) {
	reflog, err := g.run(ctx, path, "reflog", "show", "--format=%gs", "refs/heads/"+branch, "--")
	if err != nil {
		return false, err
	}
	lines := strings.Split(reflog, "\n")
	return strings.HasPrefix(lines[len(lines)-1], "clone: "), nil
}

func hasUnpushed(ctx context.Context, g gitRunner, path, rev string, pushed ...string) (bool, error) {
	n, err := g.run(ctx, path, append([]string{"rev-list", "--count", rev, "--not"}, pushed...)...)
	return n != "0", err
}

func exists(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
