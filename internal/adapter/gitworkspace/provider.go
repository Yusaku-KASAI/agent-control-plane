package gitworkspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

	if err := keepWorkRecords(ctx, g, dest); err != nil {
		return domain.Workspace{}, err
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

// targetRef keeps the target's checkout the workspace was cloned from. Its commits are the
// target's, not the session's, even when origin lacks them.
const targetRef = "refs/acp/target"

// keepWorkRecords marks the target's checkout and keeps the reflogs of remote-tracking branches,
// which tell what the session pushed, past any retention period.
func keepWorkRecords(ctx context.Context, g gitRunner, repo string) error {
	if _, err := g.run(ctx, repo, "update-ref", targetRef, "HEAD"); err != nil {
		return err
	}
	for _, key := range []string{"reflogExpire", "reflogExpireUnreachable"} {
		if _, err := g.run(ctx, repo, "config", "gc.refs/remotes/*."+key, "never"); err != nil {
			return err
		}
	}
	return nil
}

// Branches tells the branches the session pushed, by the remote-tracking branches a push updated,
// and the local branches with whether they hold commits found nowhere else.
func (p *Provider) Branches(ctx context.Context, path string) (usecase.WorkspaceBranches, error) {
	var w usecase.WorkspaceBranches
	if ok, err := exists(filepath.Join(path, ".git")); !ok || err != nil {
		return w, err
	}
	g := gitRunner{}
	remotes, err := g.run(ctx, path, "for-each-ref", "--format=%(refname)%09%(objectname)", "refs/remotes/origin")
	if err != nil {
		return w, err
	}
	tips := make(map[string]string)
	var refs []string
	for line := range strings.Lines(remotes) {
		ref, tip, _ := strings.Cut(strings.TrimSpace(line), "\t")
		if ref != "refs/remotes/origin/HEAD" {
			tips[ref] = tip
			refs = append(refs, ref)
		}
	}
	if len(refs) > 0 {
		reflogs, err := g.run(ctx, path, append(append([]string{"log", "--walk-reflogs", "--format=%gD%x09%gs"}, refs...), "--")...)
		if err != nil {
			return w, err
		}
		pushed := make(map[string]bool)
		for line := range strings.Lines(reflogs) {
			entry, subject, _ := strings.Cut(strings.TrimSpace(line), "\t")
			ref, _, _ := strings.Cut(entry, "@{")
			if subject == "update by push" && !pushed[ref] {
				pushed[ref] = true
				w.Pushed = append(w.Pushed, usecase.BranchRef{Name: strings.TrimPrefix(ref, "refs/remotes/origin/"), LocalTip: tips[ref]})
			}
		}
		slices.SortFunc(w.Pushed, func(a, b usecase.BranchRef) int { return strings.Compare(a.Name, b.Name) })
	}
	locals, err := g.run(ctx, path, "for-each-ref", "--format=%(refname:lstrip=2)%09%(objectname)", "refs/heads")
	if err != nil {
		return w, err
	}
	for line := range strings.Lines(locals) {
		name, tip, _ := strings.Cut(strings.TrimSpace(line), "\t")
		unpushed, err := hasUnpushed(ctx, g, path, "refs/heads/"+name)
		if err != nil {
			return w, err
		}
		w.Local = append(w.Local, usecase.LocalBranch{Name: name, Tip: tip, Unpushed: unpushed})
	}
	if _, err := g.run(ctx, path, "symbolic-ref", "--quiet", "HEAD"); err != nil {
		detached, err := hasUnpushed(ctx, g, path, "HEAD", "--branches")
		if err != nil {
			return w, err
		}
		if detached {
			w.Local = append(w.Local, usecase.LocalBranch{Name: "HEAD", Unpushed: true})
		}
	}
	return w, nil
}

func hasUnpushed(ctx context.Context, g gitRunner, path, rev string, elsewhere ...string) (bool, error) {
	args := append([]string{"rev-list", "--count", rev, "--not", "--remotes", "--glob=refs/acp/*"}, elsewhere...)
	n, err := g.run(ctx, path, args...)
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
