#!/usr/bin/env bash
# Checks the context scripts/context injects: how the agent is run, and the paths of the harness contract and of
# the index of platform's docs, which must exist; the output must name the hook's event.
set -euo pipefail

plugin=$(cd "$(dirname "$0")/.." && pwd)
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

# The context the plugin at $1 injects. Called in an assignment, so that set -e stops the test when it fails.
context() {
  local out
  if ! out=$("$1/scripts/context" SessionStart | jq -r .hookSpecificOutput.additionalContext); then
    echo "FAIL scripts/context of $1 failed" >&2
    return 1
  fi
  printf '%s\n' "$out"
}

# The paths the context tells: the harness contract, then the index of platform's docs.
told() { sed -n -e 's/^ハーネスの契約（[^）]*）[^：]*：//p' -e 's/^platform の文書の一覧（[^）]*）：//p' <<<"$1"; }

# The told paths that are not files.
missing() { told "$1" | while read -r f; do [[ -f $f ]] || echo "$f"; done; }

docs=$(cd "$plugin/../../docs" && pwd)
ctx=$(ACP_SESSION_BRANCH=acp/test context "$plugin")
check "the context tells the contract and the index beside the plugins" "$docs/reference/harness.md
$docs/README.md" "$(told "$ctx")"
check "every path the context tells exists" "" "$(missing "$ctx")"
check "under the control plane, it tells so and names the session's branch" "動き方：control-plane 経由 acp/test" \
  "$(grep -o '^動き方：.*' <<<"$ctx") $(grep -o 'acp/test' <<<"$ctx" | head -1)"
check "run directly, it tells so" "動き方：devcontainer で直接" \
  "$(env -u ACP_SESSION_BRANCH "$plugin/scripts/context" SessionStart | jq -r .hookSpecificOutput.additionalContext |
    grep -o '^動き方：.*')"
check "the output names the event it was called for" "SubagentStart" \
  "$("$plugin/scripts/context" SubagentStart | jq -r .hookSpecificOutput.hookEventName)"

# Copies of the plugin in a marketplace of their own, and outside any.
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/market/plugins" "$tmp/market/docs/reference" "$tmp/elsewhere"
cp -R "$plugin" "$tmp/market/plugins/harness"
cp -R "$plugin" "$tmp/elsewhere/harness"
touch "$tmp/market/docs/README.md" "$tmp/market/docs/reference/harness.md"
check "a copy in another marketplace tells the docs beside its plugins" "$tmp/market/docs/reference/harness.md
$tmp/market/docs/README.md" "$(told "$(context "$tmp/market/plugins/harness")")"
rm "$tmp/market/docs/reference/harness.md"
check "a marketplace without the contract is told as not found" "（見つからない。platform の docs/）/reference/harness.md
（見つからない。platform の docs/）/README.md" "$(told "$(context "$tmp/market/plugins/harness")")"
# Outside a marketplace, the docs are looked for where every target declares the marketplace.
fallback=/workspace/agent-platform/docs
[[ -f $fallback/reference/harness.md ]] || fallback="（見つからない。platform の docs/）"
check "a copy outside a marketplace looks where targets declare it" "$fallback/reference/harness.md
$fallback/README.md" "$(told "$(context "$tmp/elsewhere/harness")")"

exit "$failed"
