#!/bin/sh
# Warns of a new remote branch outside the session's names, stops a push that would overwrite a
# remote tip no reflog entry of the pushed ref reaches unless ACP_PUSH_OVERWRITE=1 says the
# overwrite is meant, then runs the target's checks, make check. Neither the warning nor the stop
# forbids anything: a requester may ask for another name or for the overwrite. The checks run on
# the work tree, so they stand for the pushed commit only when that commit is HEAD and the work
# tree has nothing beside it. Like commit-msg, it lives only in the workspace's .git.

# The control plane writes the session branch here on installing the hook.
session_branch='@SESSION_BRANCH@'
say() { echo "acp pre-push: $*" >&2; }
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
