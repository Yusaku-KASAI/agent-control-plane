#!/usr/bin/env bash
# Checks that the guard-tree hook keeps subagents from changing the work area under the control
# plane, and tells the agent when the work area changed while a subagent ran.
set -euo pipefail

hook=$(cd "$(dirname "$0")/.." && pwd)/scripts/guard-tree
failed=0
check() {
  local name=$1 want=$2 got=$3
  if [[ $got == "$want" ]]; then
    echo "ok   $name"
  else
    echo "FAIL $name: got '$got', want '$want'"
    failed=1
  fi
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
repo=$tmp/repo
git init --quiet -b main "$repo"
g() { git -C "$repo" -c user.name=t -c user.email=t@example.com "$@"; }
echo v1 >"$repo/README.md"
echo build/ >"$repo/.gitignore"
g add README.md .gitignore
g commit --quiet -m v1
echo v2 >"$repo/README.md"
g commit --quiet -am v2
linked=$tmp/linked
g worktree add --quiet -b linked "$linked"

export ACP_SESSION_BRANCH=acp/x CLAUDE_PROJECT_DIR=$repo

# run <agent id or ""> <event> <tool> <tool input> [cwd]: the hook's output.
run() {
  jq -nc --arg agent "$1" --arg event "$2" --arg tool "$3" --argjson input "$4" --arg cwd "${5:-$repo}" \
    '{session_id: "s", cwd: $cwd, hook_event_name: $event, tool_name: $tool, tool_input: $input,
      tool_use_id: "toolu_1"} + (if $agent == "" then {} else {agent_id: $agent} end)' | "$hook"
}
decision() { jq -r '.hookSpecificOutput.permissionDecision // ""' <<<"$(cat)"; }
bash_by() { run "$1" PreToolUse Bash "$(jq -nc --arg c "$2" '{command: $c}')" "${3:-$repo}" | decision; }
write_by() { run "$1" PreToolUse "${3:-Write}" "$(jq -nc --arg p "$2" '{file_path: $p, notebook_path: $p, content: ""}')" | decision; }

# Writing files.
check "subagent writing in the work area is denied" deny "$(write_by a1 "$repo/x.txt")"
check "subagent editing in the work area is denied" deny "$(write_by a1 "$repo/README.md" Edit)"
check "subagent editing a notebook in the work area is denied" deny "$(write_by a1 "$repo/n.ipynb" NotebookEdit)"
check "subagent writing in a new directory of the work area is denied" deny "$(write_by a1 "$repo/new/dir/x.txt")"
check "subagent writing a relative path is denied" deny "$(write_by a1 "x.txt")"
check "subagent writing outside the work area is left alone" "" "$(write_by a1 "$tmp/scratch/x.txt")"
check "subagent writing in a linked worktree is left alone" "" "$(write_by a1 "$linked/x.txt")"
check "the agent writing in the work area is left alone" "" "$(write_by "" "$repo/x.txt")"
check "subagent writing is left alone outside the control plane" "" \
  "$(ACP_SESSION_BRANCH='' write_by a1 "$repo/x.txt")"

# git commands.
for c in 'git checkout HEAD~1' 'git switch -c other' 'git stash' 'git stash push -m m' 'git add -A' \
  'git commit -m m' 'git reset --hard' 'git restore README.md' 'git rebase main' 'git clean -fd' \
  'git apply p.diff' 'git -c core.pager=cat checkout main' 'git log | head; git reset HEAD~1' \
  'cd sub && git checkout main' 'make check && git add README.md'; do
  check "subagent: $c is denied" deny "$(bash_by a1 "$c")"
done
for c in 'git status' 'git diff --stat origin/main...HEAD' 'git log --oneline' 'git show HEAD~1:README.md' \
  'git stash list' 'git stash show -p' 'git apply --check p.diff' 'git apply --stat p.diff' \
  'echo "git checkout main"' "grep -n 'git reset' README.md" 'git -C /tmp/wt checkout main' \
  'git --git-dir=/tmp/wt/.git reset' 'go test ./...' 'cat <<EOF
git checkout main
EOF'; do
  check "subagent: $c is left alone" "" "$(bash_by a1 "$c")"
done
check "subagent: git checkout in a linked worktree is left alone" "" "$(bash_by a1 'git checkout main' "$linked")"
check "the agent: git checkout is left alone" "" "$(bash_by "" 'git checkout main')"
check "subagent: git checkout is left alone outside the control plane" "" \
  "$(ACP_SESSION_BRANCH='' bash_by a1 'git checkout main')"

# Comparing the work area around a subagent. around <change>: the context after the change.
around() {
  local out
  out=$(run "" PreToolUse Agent '{"prompt":"p"}')
  [[ -z $out ]] || echo "unexpected output before the subagent: $out"
  (cd "$repo" && eval "$1")
  run "" PostToolUse Agent '{"prompt":"p"}' | jq -r '.hookSpecificOutput.additionalContext // ""'
}
snapshot=$(git -C "$repo" rev-parse --path-format=absolute --git-path acp-subagent-toolu_1)
check "no change, nothing to tell" "" "$(around ':')"
check "the snapshot is removed" no "$([[ -e $snapshot ]] && echo yes || echo no)"
contains() { if [[ $2 == *"$1"* ]]; then echo yes; else echo "no: $2"; fi; }
check "an untracked file is told" yes "$(contains '今：ファイル' "$(around 'touch "ait; } => |"')")"
rm -f "$repo/ait; } => |"
echo dirty >"$repo/README.md"
check "a modified file changed again is told" yes "$(contains 'README.md' "$(around 'echo dirtier >README.md')")"
check "staging is told" yes "$(contains '今：index' "$(around 'git add README.md')")"
g reset --quiet --hard
check "a detached HEAD is told" yes "$(contains '今：ブランチ (detached)' "$(around 'git checkout --quiet HEAD~1')")"
g switch --quiet main
check "a stash is told" yes "$(contains '今：stash' "$(around 'echo s >README.md; git stash --quiet')")"
check "an ignored file is not told" "" "$(around 'mkdir -p build && touch build/out')"
out=$(run "" PreToolUse Agent '{"prompt":"p"}')
touch "$repo/failed"
check "a change around a failed subagent is told" yes "$(contains 'failed' \
  "$(run "" PostToolUseFailure Agent '{"prompt":"p"}' | jq -r '.hookSpecificOutput.additionalContext // ""')")"
rm -f "$repo/failed"
check "a subagent's own Agent call takes no snapshot" "" "$(run a1 PreToolUse Agent '{"prompt":"p"}')"
check "without a snapshot, nothing to tell" "" "$(run "" PostToolUse Agent '{"prompt":"p"}')"
exit $failed
