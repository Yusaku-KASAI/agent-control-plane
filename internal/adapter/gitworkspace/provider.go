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
	if err := installHooks(dest, id); err != nil {
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

// prePushHook warns of a new remote branch outside the session's names, stops a push that would
// overwrite a remote tip no reflog entry of the pushed ref reaches unless ACP_PUSH_OVERWRITE=1 says
// the overwrite is meant, then runs the target's checks, make check. Neither the warning nor the
// stop forbids anything: a requester may ask for another name or for the overwrite. The checks run
// on the work tree, so they stand for the pushed commit only when that commit is HEAD and the work
// tree has nothing beside it. Like commitMsgHook, it lives only in the workspace's .git.
func prePushHook(id domain.SessionID) string {
	return "#!/bin/sh\nsession_branch='" + id.Branch() + "'\n" + prePushBody
}

const prePushBody = `say() { echo "acp pre-push: $*" >&2; }
refs=$(cat)
while read -r _ local_sha remote_ref remote_sha; do
	case $local_sha in *[!0]*) ;; *) continue ;; esac
	case $remote_sha in *[!0]*) continue ;; esac
	case $remote_ref in refs/heads/*) ;; *) continue ;; esac
	name=${remote_ref#refs/heads/}
	case $name in "$session_branch" | "$session_branch"-*) continue ;; esac
	say "注意：新しいブランチ $name は、セッションが作ったブランチの名前（$session_branch か $session_branch-<内容>）ではない。既存の PR を続けるならその PR のブランチで、別の PR に分けるなら $session_branch-<内容> で作業する（ハーネスの契約の「作業領域とブランチ」）。名前のためには push を止めない。合っていないのにこの push が通ったなら、正しいブランチで push し直し、このブランチは git push $1 --delete $name で消す（片付けはセッションが作ったブランチしか消さない）。"
done <<REFS
$refs
REFS
[ "${ACP_PUSH_OVERWRITE:-}" = 1 ] || while read -r local_ref local_sha remote_ref remote_sha; do
	case $local_sha in *[!0]*) ;; *) continue ;; esac
	case $remote_sha in *[!0]*) ;; *) continue ;; esac
	git merge-base --is-ancestor "$remote_sha" "$local_sha" 2>/dev/null && continue
	# HEAD's own reflog also holds what was only checked out to look at; only a branch or tag named
	# as such has a reflog to read.
	if [ "$local_ref" = HEAD ]; then
		local_ref=$(git symbolic-ref -q HEAD)
	fi
	case $local_ref in refs/heads/* | refs/tags/*) ;; *) local_ref= ;; esac
	[ -n "$local_ref" ] && for c in $(git reflog show --format=%H "$local_ref" -- 2>/dev/null); do
		git merge-base --is-ancestor "$remote_sha" "$c" 2>/dev/null && continue 2
	done
	say "注意：この push は $remote_ref のリモートの先頭 $remote_sha を、手元に取り込まないまま上書きする（ほかの書き手のコミットが消える）。ふつうは取り込んでから（git pull --rebase）push し直す。上書きすると決めたときだけ、ACP_PUSH_OVERWRITE=1 と --force を付けて push し直す（ハーネスの契約の「push」）。"
	exit 1
done <<REFS
$refs
REFS
top=$(git rev-parse --show-toplevel) || exit 1
if ! [ -f "$top/Makefile" ] && ! [ -f "$top/makefile" ] && ! [ -f "$top/GNUmakefile" ]; then
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
done <<REFS
$refs
REFS
[ -n "$pushes" ] || exit 0
if [ -n "$(git status --porcelain --untracked-files=all)" ]; then
	say "作業ツリーが HEAD とずれている。変更に入れるものはコミットし、ほかは消してから push する："
	git status --short >&2
	exit 1
fi
# The checks may run git in repositories of their own, so the variables git sets for a hook (in a
# linked worktree, GIT_DIR) must not point them here. The configuration in GIT_CONFIG_* stays.
unset ACP_PUSH_OVERWRITE GIT_DIR GIT_WORK_TREE GIT_COMMON_DIR GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX
say "make check"
if ! make -C "$top" check </dev/null >&2; then
	say "make check が落ちた。直してコミットし、push し直す。"
	exit 1
fi
`

func installHooks(repo string, id domain.SessionID) error {
	dir := filepath.Join(repo, ".git", "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, body := range map[string]string{"commit-msg": commitMsgHook, "pre-push": prePushHook(id)} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			return err
		}
	}
	return nil
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

// State tells the working trees, the branches the session pushed, by the remote-tracking branches a
// push updated, and the local branches with whether they hold commits found nowhere else. The
// working trees include the linked worktrees inside the workspace, such as those of subagents.
func (p *Provider) State(ctx context.Context, path string) (usecase.WorkspaceState, error) {
	var w usecase.WorkspaceState
	if ok, err := exists(filepath.Join(path, ".git")); !ok || err != nil {
		return w, err
	}
	g := gitRunner{}
	linked, err := linkedWorktrees(path)
	if err != nil {
		return w, err
	}
	if err := readWorkingTree(ctx, g, path, &w); err != nil {
		return w, err
	}
	for _, wt := range linked {
		if wt.dir == "" {
			continue
		}
		if err := readWorkingTree(ctx, g, path, &w, "--git-dir="+wt.gitDir, "--work-tree="+wt.dir); err != nil {
			return w, err
		}
	}
	if _, err := g.run(ctx, path, "rev-parse", "--verify", "--quiet", "refs/stash"); err == nil {
		w.Stash = true
	}
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
	// A worktree outside the workspace, such as one prctl makes to read a pull request, holds
	// others' commits on its detached HEAD; its files are gone with the container anyway.
	heads := [][]string{nil}
	for _, wt := range linked {
		if wt.dir != "" {
			heads = append(heads, []string{"--git-dir=" + wt.gitDir})
		}
	}
	for _, gitDir := range heads {
		tip, err := detachedTip(ctx, g, path, gitDir...)
		if err != nil {
			return w, err
		}
		if tip != "" {
			w.Local = append(w.Local, usecase.LocalBranch{BranchTip: usecase.BranchTip{Name: "HEAD", Tip: tip}, Unpushed: true})
		}
	}
	return w, nil
}

// detachedTip returns HEAD when it is detached on commits no branch has, or "" otherwise. gitDir
// selects the HEAD of a linked worktree.
func detachedTip(ctx context.Context, g gitRunner, path string, gitDir ...string) (string, error) {
	if _, err := g.run(ctx, path, append(gitDir, "symbolic-ref", "--quiet", "HEAD")...); err == nil {
		return "", nil
	}
	n, err := g.run(ctx, path, append(gitDir, "rev-list", "--count", "HEAD", "--not", "--remotes=origin", "--glob=refs/acp/*", "--branches")...)
	if err != nil || n == "0" {
		return "", err
	}
	return g.run(ctx, path, append(gitDir, "rev-parse", "HEAD")...)
}

// worktree is a linked worktree of the workspace: its administrative directory in the
// repository's .git, and its files as seen here, or "" when they are not inside the workspace.
type worktree struct {
	gitDir string
	dir    string
}

// linkedWorktrees lists the linked worktrees. Git records their paths as the agent's container
// saw them, which differ from the paths here, so git worktree list cannot locate them.
func linkedWorktrees(path string) ([]worktree, error) {
	admin := filepath.Join(path, ".git", "worktrees")
	entries, err := os.ReadDir(admin)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var wts []worktree
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		wt := worktree{gitDir: filepath.Join(admin, e.Name())}
		// Without HEAD, git does not take the directory for a worktree, such as what a stopped
		// git worktree add leaves.
		if ok, err := exists(filepath.Join(wt.gitDir, "HEAD")); err != nil {
			return nil, err
		} else if !ok {
			continue
		}
		recorded, err := os.ReadFile(filepath.Join(wt.gitDir, "gitdir"))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if err == nil {
			wt.dir = locateWorktree(path, filepath.Dir(strings.TrimSpace(string(recorded))), e.Name())
		}
		wts = append(wts, wt)
	}
	return wts, nil
}

// locateWorktree finds where the files of the linked worktree name are, from the path git
// recorded: the longest tail of it that, under path, holds a .git file pointing back to the
// worktree's administrative directory. The agent can write the record, so the place found must
// stay inside path once .. and symbolic links are resolved.
func locateWorktree(path, recorded, name string) string {
	root, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(recorded), "/")
	for i := range parts {
		tail := parts[i:]
		if strings.Join(tail, "") == "" {
			continue
		}
		dir, err := filepath.EvalSymlinks(filepath.Join(path, filepath.FromSlash(strings.Join(tail, "/"))))
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(root, dir); err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
			continue
		}
		link, err := os.ReadFile(filepath.Join(dir, ".git"))
		if err != nil {
			continue
		}
		target := filepath.ToSlash(strings.TrimSpace(strings.TrimPrefix(string(link), "gitdir:")))
		if strings.HasSuffix(target, "/.git/worktrees/"+name) {
			return dir
		}
	}
	return ""
}

// operations are the files git keeps while an operation is stopped halfway.
var operations = []struct{ file, name string }{
	{"rebase-merge", "rebase"}, {"rebase-apply", "rebase"}, {"MERGE_HEAD", "merge"},
	{"CHERRY_PICK_HEAD", "cherry-pick"}, {"REVERT_HEAD", "revert"}, {"BISECT_LOG", "bisect"},
}

// readWorkingTree adds what one working tree holds to w. gitArgs select a linked worktree.
func readWorkingTree(ctx context.Context, g gitRunner, path string, w *usecase.WorkspaceState, gitArgs ...string) error {
	run := func(args ...string) (string, error) {
		return g.run(ctx, path, append(slices.Clone(gitArgs), args...)...)
	}
	status, err := run("status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return err
	}
	w.Changes = w.Changes || status != ""
	if w.Operation != "" {
		return nil
	}
	for _, op := range operations {
		file, err := run("rev-parse", "--git-path", op.file)
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
