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
# Only .* around the wide characters, so that it reads the same in a locale that is not UTF-8.
told() { sed -n -e 's/^ハーネスの契約.*）：//p' -e 's/^platform の文書の一覧.*）：//p' <<<"$1"; }

# Each event the hooks run scripts/context for must reach the output by its name.
wired() {
  local plugin=$1 event command
  while IFS=$'\t' read -r event command; do
    echo "$event $(CLAUDE_PLUGIN_ROOT=$plugin bash -c "$command" </dev/null | jq -r .hookSpecificOutput.hookEventName)"
  done < <(jq -r '.hooks | to_entries[] | .key as $e | .value[].hooks[].command | select(test("scripts/context"))
    | [$e, .] | @tsv' "$plugin/hooks/hooks.json")
}

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
check "the hooks run it at the start of sessions and subagents, and the output names the event" \
  "SessionStart SessionStart
SubagentStart SubagentStart" "$(wired "$plugin")"

# Copies of the plugin in a marketplace of their own, and outside any.
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/market/plugins" "$tmp/market/docs/reference" "$tmp/elsewhere"
cp -R "$plugin" "$tmp/market/plugins/harness"
cp -R "$plugin" "$tmp/elsewhere/harness"
touch "$tmp/market/docs/README.md" "$tmp/market/docs/reference/harness.md"
out=$(context "$tmp/market/plugins/harness")
check "a copy in another marketplace tells the docs beside its plugins" "$tmp/market/docs/reference/harness.md
$tmp/market/docs/README.md" "$(told "$out")"
rm "$tmp/market/docs/reference/harness.md"
out=$(context "$tmp/market/plugins/harness")
check "a marketplace without the contract is told as not found" "（見つからない。platform の docs/）/reference/harness.md
（見つからない。platform の docs/）/README.md" "$(told "$out")"
# Outside a marketplace, the docs are looked for where every target declares the marketplace. That place is
# moved into the temporary directory in the copy: a script that names another place is left unchanged and fails.
sed -i "s#/workspace/agent-platform/docs#$tmp/declared/docs#" "$tmp/elsewhere/harness/scripts/context"
mkdir -p "$tmp/declared/docs/reference"
touch "$tmp/declared/docs/reference/harness.md"
out=$(context "$tmp/elsewhere/harness")
check "a copy outside a marketplace looks where targets declare it" "$tmp/declared/docs/reference/harness.md
$tmp/declared/docs/README.md" "$(told "$out")"
rm "$tmp/declared/docs/reference/harness.md"
out=$(context "$tmp/elsewhere/harness")
check "a copy outside a marketplace without the contract there is told as not found" \
  "（見つからない。platform の docs/）/reference/harness.md
（見つからない。platform の docs/）/README.md" "$(told "$out")"

exit "$failed"
