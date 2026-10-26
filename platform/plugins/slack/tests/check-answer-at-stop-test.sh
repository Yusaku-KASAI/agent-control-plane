#!/usr/bin/env bash
# Checks that the Stop hook finds, under control-plane, the forms Slack reads differently in the
# turn's last message, and holds the turn once for the same findings.
set -euo pipefail

plugin=$(cd "$(dirname "$0")/.." && pwd)
hook=$plugin/scripts/check-answer-at-stop
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
export TMPDIR=$tmp ACP_SESSION_BRANCH=acp/s1

# stop <session> <answer>: what the hook decides at the end of a turn that gave the answer, once a
# Stop hook has kept it going; first <session> <answer>: at its first end.
ask() { jq -n --arg s "$1" --arg a "$2" --argjson c "$3" '{session_id: $s, last_assistant_message: $a, stop_hook_active: $c}' | "$hook" | jq -r '.decision // empty'; }
stop() { ask "$1" "$2" true; }
first() { ask "$1" "$2" false; }
broken='PR：https://x.example'
fine='PR：<https://x.example>'

check "an answer Slack reads alike lets the turn end" "" "$(stop s1 "$fine")"
check "an answer with a form Slack reads differently holds the turn" block "$(first s1 "$broken")"
reason=$(jq -n --arg a "$broken" '{session_id: "s9", last_assistant_message: $a}' | "$hook" | jq -r .reason)
check "the reason names the line and the form" yes "$([[ $reason == *"line 1: URL"* ]] && echo yes || echo no)"
check "the same findings hold the turn once" "" "$(stop s1 "$broken")"
check "the same findings on other lines hold the turn once" "" "$(stop s1 $'書き直しました。\n\n'"$broken")"
check "other findings hold the turn again" block "$(stop s1 $'| a |\n| --- |\n文\n本文\n| b |\n| --- |')"
two=$'本文\n| a |\n| --- |\n'"$broken"
check "two forms hold the turn" block "$(first s8 "$two")"
check "one of them rewritten, the other kept, holds the turn once" "" "$(stop s8 "$broken")"
check "a form added to those that held the turn holds it again" block "$(stop s8 "$two"$'\n\n# **見出し**')"
check "another session is held for the same findings" block "$(stop s2 "$broken")"
stop s2 "$fine" >/dev/null
check "after an answer that passes, the same findings hold the turn again" block "$(stop s2 "$broken")"
check "the next turn is held for the same findings" block "$(first s2 "$broken")"
check "a turn without its last message is not held" "" "$(jq -n '{session_id: "s3"}' | "$hook" | jq -r '.decision // empty')"
first s7 "$broken" >/dev/null
jq -n '{session_id: "s7", last_assistant_message: "", stop_hook_active: true}' | "$hook" >/dev/null
check "after an empty answer, the same findings hold the turn again" block "$(stop s7 "$broken")"
check "outside control-plane, the answer is not checked" "" "$(ACP_SESSION_BRANCH='' stop s4 "$broken")"

mkdir -p "$tmp/broken"
printf '#!/usr/bin/env bash\nexit 1\n' >"$tmp/broken/python3"
chmod +x "$tmp/broken/python3"
check "a failing check-markdown does not hold the turn" "" "$(PATH=$tmp/broken:$PATH stop s5 "$broken")"
mkdir -p "$tmp/readonly"
chmod 500 "$tmp/readonly"
check "findings that cannot be remembered do not hold the turn" "" "$(TMPDIR=$tmp/readonly stop s6 "$broken")"

check "the hook is called at Stop" "$hook" \
  "$(jq -r '.hooks.Stop[].hooks[].command' "$plugin/hooks/hooks.json" | sed "s|\"\${CLAUDE_PLUGIN_ROOT}\"|$plugin|")"

exit "$failed"
