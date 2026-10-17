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
	if err := installHooks(dest); err != nil {
		return domain.Workspace{}, err
	}
	if err := excludeAgentWorktrees(dest); err != nil {
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

// prePushHook runs the target's checks, make check, before a push. The checks run on the work
// tree, so they stand for the pushed commit only when that commit is HEAD and the work tree has
// nothing beside it. Like commitMsgHook, it lives only in the workspace's .git.
const prePushHook = `#!/bin/sh
say() { echo "acp pre-push: $*" >&2; }
top=$(git rev-parse --show-toplevel) || exit 1
if ! [ -f "$top/Makefile" ] && ! [ -f "$top/makefile" ] && ! [ -f "$top/GNUmakefile" ]; then
	cat >/dev/null
	exit 0
fi
head=$(git rev-parse --verify HEAD) || exit 1
pushes=
while read -r local_ref local_sha _; do
	case $local_sha in *[!0]*) ;; *) continue ;; esac
	commit=$(git rev-parse --verify --quiet "$local_sha^{commit}") || commit=$local_sha
	if [ "$commit" != "$head" ]; then
		say "$local_ref が今の HEAD ではない。make check は作業ツリーで回すので、push するコミットを HEAD にしてから push する。"
		exit 1
	fi
	pushes=yes
done
[ -n "$pushes" ] || exit 0
if [ -n "$(git status --porcelain --untracked-files=all)" ]; then
	say "作業ツリーが HEAD とずれている。変更に入れるものはコミットし、ほかは消してから push する："
	git status --short >&2
	exit 1
fi
# The checks may run git in repositories of their own, so the variables git sets for a hook (in a
# linked worktree, GIT_DIR) must not point them here. The configuration in GIT_CONFIG_* stays.
unset GIT_DIR GIT_WORK_TREE GIT_COMMON_DIR GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX
say "make check"
if ! make -C "$top" check </dev/null >&2; then
	say "make check が落ちた。直してコミットし、push し直す。"
	exit 1
fi
`

func installHooks(repo string) error {
	dir := filepath.Join(repo, ".git", "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, body := range map[string]string{"commit-msg": commitMsgHook, "pre-push": prePushHook} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// agentWorktrees is where Claude Code puts the worktrees of subagents run with isolation: worktree.
// Kept there, a worktree with changes would show as untracked files of the workspace, and prePushHook
// would refuse every push. Excluding it in the workspace's .git leaves the target's .gitignore alone.
const agentWorktrees = "/.claude/worktrees/"

func excludeAgentWorktrees(repo string) error {
	path := filepath.Join(repo, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.WriteString("\n" + agentWorktrees + "\n")
	return errors.Join(err, f.Close())
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
		// git status would otherwise refresh the index, racing with an agent using the workspace.
		"GIT_OPTIONAL_LOCKS=0",
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

// State tells the working tree, the branches the session pushed, by the remote-tracking branches a
// push updated, and the local branches with whether they hold commits found nowhere else.
func (p *Provider) State(ctx context.Context, path string) (usecase.WorkspaceState, error) {
	var w usecase.WorkspaceState
	if ok, err := exists(filepath.Join(path, ".git")); !ok || err != nil {
		return w, err
	}
	g := gitRunner{}
	if err := readWorkingTree(ctx, g, path, &w); err != nil {
		return w, err
	}
	var err error
	if w.Pushed, err = pushedBranches(ctx, g, path); err != nil {
		return w, err
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
		w.Local = append(w.Local, usecase.LocalBranch{BranchTip: usecase.BranchTip{Name: name, Tip: tip}, Unpushed: unpushed})
	}
	if _, err := g.run(ctx, path, "symbolic-ref", "--quiet", "HEAD"); err != nil {
		detached, err := hasUnpushed(ctx, g, path, "HEAD", "--branches")
		if err != nil {
			return w, err
		}
		if detached {
			tip, err := g.run(ctx, path, "rev-parse", "HEAD")
			if err != nil {
				return w, err
			}
			w.Local = append(w.Local, usecase.LocalBranch{BranchTip: usecase.BranchTip{Name: "HEAD", Tip: tip}, Unpushed: true})
		}
	}
	return w, nil
}

// operations are the files git keeps while an operation is stopped halfway.
var operations = []struct{ file, name string }{
	{"rebase-merge", "rebase"}, {"rebase-apply", "rebase"}, {"MERGE_HEAD", "merge"},
	{"CHERRY_PICK_HEAD", "cherry-pick"}, {"REVERT_HEAD", "revert"}, {"BISECT_LOG", "bisect"},
}

func readWorkingTree(ctx context.Context, g gitRunner, path string, w *usecase.WorkspaceState) error {
	status, err := g.run(ctx, path, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return err
	}
	w.Changes = status != ""
	if _, err := g.run(ctx, path, "rev-parse", "--verify", "--quiet", "refs/stash"); err == nil {
		w.Stash = true
	}
	for _, op := range operations {
		file, err := g.run(ctx, path, "rev-parse", "--git-path", op.file)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(file) {
			file = filepath.Join(path, file)
		}
		if ok, err := exists(file); err != nil {
			return err
		} else if ok {
			w.Operation = op.name
			break
		}
	}
	return nil
}

func pushedBranches(ctx context.Context, g gitRunner, path string) ([]usecase.BranchTip, error) {
	remotes, err := g.run(ctx, path, "for-each-ref", "--format=%(refname)", "refs/remotes/origin")
	if err != nil {
		return nil, err
	}
	var refs []string
	for line := range strings.Lines(remotes) {
		if ref := strings.TrimSpace(line); ref != "refs/remotes/origin/HEAD" {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return nil, nil
	}
	reflogs, err := g.run(ctx, path, append(append([]string{"log", "--walk-reflogs", "--format=%gD%x09%H%x09%gs"}, refs...), "--")...)
	if err != nil {
		return nil, err
	}
	var pushed []usecase.BranchTip
	seen := make(map[string]bool)
	// The newest entry comes first. Its commit, not the remote-tracking branch now, is what the
	// session pushed: a fetch may have moved the branch past it or away from it.
	for line := range strings.Lines(reflogs) {
		fields := strings.SplitN(strings.TrimSpace(line), "\t", 3)
		if len(fields) != 3 {
			continue
		}
		ref, _, _ := strings.Cut(fields[0], "@{")
		if fields[2] == "update by push" && !seen[ref] {
			seen[ref] = true
			pushed = append(pushed, usecase.BranchTip{Name: strings.TrimPrefix(ref, "refs/remotes/origin/"), Tip: fields[1]})
		}
	}
	slices.SortFunc(pushed, func(a, b usecase.BranchTip) int { return strings.Compare(a.Name, b.Name) })
	return pushed, nil
}

func hasUnpushed(ctx context.Context, g gitRunner, path, rev string, elsewhere ...string) (bool, error) {
	args := append([]string{"rev-list", "--count", rev, "--not", "--remotes=origin", "--glob=refs/acp/*"}, elsewhere...)
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
