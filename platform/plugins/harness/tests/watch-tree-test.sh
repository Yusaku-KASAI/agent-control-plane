#!/usr/bin/env bash
# Checks that the watch-tree hook tells the agent, under the control plane, when the work area
# changed while a subagent ran.
set -euo pipefail

hook=$(cd "$(dirname "$0")/.." && pwd)/scripts/watch-tree
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
check "a linked worktree made in the work area is not told" "" \
  "$(around 'git worktree add --quiet -b inner .claude/worktrees/agent-1 && touch .claude/worktrees/agent-1/work')"
check "an ignored file is not told" "" "$(around 'mkdir -p build && touch build/out')"
out=$(run "" PreToolUse Agent '{"prompt":"p"}')
touch "$repo/failed"
check "a change around a failed subagent is told" yes "$(contains 'failed' \
  "$(run "" PostToolUseFailure Agent '{"prompt":"p"}' | jq -r '.hookSpecificOutput.additionalContext // ""')")"
rm -f "$repo/failed"
stale=$(dirname "$snapshot")/acp-subagent-toolu_0
touch -d '2 days ago' "$stale"
run "" PreToolUse Agent '{"prompt":"p"}' >/dev/null
run "" PostToolUse Agent '{"prompt":"p"}' >/dev/null
check "a snapshot left for a day is removed" no "$([[ -e $stale ]] && echo yes || echo no)"
check "a subagent's own Agent call takes no snapshot" "" "$(run a1 PreToolUse Agent '{"prompt":"p"}')"
check "without a snapshot, nothing to tell" "" "$(run "" PostToolUse Agent '{"prompt":"p"}')"
check "nothing is watched outside the control plane" "" "$(ACP_SESSION_BRANCH='' run "" PreToolUse Agent '{"prompt":"p"}')"
check "outside the control plane, no snapshot is taken" no \
  "$([[ -e $snapshot ]] && echo yes || echo no)"
exit $failed
