#!/usr/bin/env bash
# Checks that the foreground hook keeps subagents in the foreground only under the control plane.
set -euo pipefail

hook=$(cd "$(dirname "$0")/.." && pwd)/scripts/foreground
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

agent='{"tool_name":"Agent","tool_input":{"description":"review","prompt":"p","run_in_background":true}}'
send='{"tool_name":"SendMessage","tool_input":{"to":"a1","message":"m"}}'
bash='{"tool_name":"Bash","tool_input":{"command":"sleep 1","run_in_background":true}}'

out=$(ACP_SESSION_BRANCH=acp/x "$hook" <<<"$agent")
check "agent runs in the foreground" false "$(jq -c .hookSpecificOutput.updatedInput.run_in_background <<<"$out")"
check "agent keeps its other input" '"p"' "$(jq -c .hookSpecificOutput.updatedInput.prompt <<<"$out")"
out=$(ACP_SESSION_BRANCH=acp/x "$hook" <<<"${agent/,\"run_in_background\":true/}")
check "agent without the flag runs in the foreground" false "$(jq -c .hookSpecificOutput.updatedInput.run_in_background <<<"$out")"
out=$(ACP_SESSION_BRANCH=acp/x "$hook" <<<"${agent/\"prompt\"/\"isolation\":\"remote\",\"prompt\"}")
check "remote agent is denied" '"deny"' "$(jq -c .hookSpecificOutput.permissionDecision <<<"$out")"
out=$(ACP_SESSION_BRANCH=acp/x "$hook" <<<"${agent/\"prompt\"/\"isolation\":\"worktree\",\"prompt\"}")
check "worktree agent runs in the foreground" false "$(jq -c .hookSpecificOutput.updatedInput.run_in_background <<<"$out")"
out=$(ACP_SESSION_BRANCH=acp/x "$hook" <<<"$send")
check "send message is denied" '"deny"' "$(jq -c .hookSpecificOutput.permissionDecision <<<"$out")"
check "bash is left alone" "" "$(ACP_SESSION_BRANCH=acp/x "$hook" <<<"$bash")"
check "agent is left alone outside the control plane" "" "$(env -u ACP_SESSION_BRANCH "$hook" <<<"$agent")"
check "send message is left alone outside the control plane" "" "$(env -u ACP_SESSION_BRANCH "$hook" <<<"$send")"
exit $failed
