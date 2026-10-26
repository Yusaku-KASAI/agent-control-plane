#!/usr/bin/env bash
# Checks that every path the injected rules link to exists: the rules link their sources relative to
# rules/, and scripts/context turns those links into absolute paths.
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

# The paths the context of a plugin at $1 links to that do not exist, one per line.
missing() {
  "$1/scripts/context" SessionStart | jq -r .hookSpecificOutput.additionalContext \
    | grep -o '](/[^)#"]*' | cut -c3- | sed 's/ *$//' | while read -r f; do [[ -e $f ]] || echo "$f"; done
}

check "every path the rules link to exists" "" "$(missing "$plugin")"
context=$("$plugin/scripts/context" SessionStart | jq -r .hookSpecificOutput.additionalContext)
check "a path is linked at all" "yes" "$([[ $context == *"](/"* ]] && echo yes || echo no)"

# A copy whose rules link to a document that is not there must be told.
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/plugins"
cp -R "$plugin" "$tmp/plugins/conventions"
cp -R "$(dirname "$plugin")/principles" "$tmp/plugins/principles"
printf '\n- [ない文書](../docs/missing.md)\n' >>"$tmp/plugins/conventions/rules/common.md"
check "a link to a missing document is told" "$tmp/plugins/conventions/docs/missing.md" "$(missing "$tmp/plugins/conventions")"

exit "$failed"
