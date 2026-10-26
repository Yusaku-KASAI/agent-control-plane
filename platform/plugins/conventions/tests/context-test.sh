#!/usr/bin/env bash
# Checks the context scripts/context injects: the rules link their sources relative to rules/, and
# every link must reach the reader as an absolute path that exists.
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

# The context a plugin at $1 injects. Called in an assignment, so that set -e stops the test when it fails.
context() {
  local out
  if ! out=$("$1/scripts/context" SessionStart | jq -r .hookSpecificOutput.additionalContext); then
    echo "FAIL scripts/context of $1 failed" >&2
    return 1
  fi
  printf '%s\n' "$out"
}

# The targets of the links in a context, one per line.
targets() { grep -o '](\([^)#"]*\)' <<<"$1" | cut -c3- | sed 's/ *$//' || true; }

# The targets left relative: neither an absolute path nor a URL nor an anchor in the same document.
relative() { targets "$1" | grep -vE '^(/|[a-z]+:|$)' || true; }

# The absolute targets that do not exist.
missing() { targets "$1" | grep '^/' | while read -r f; do [[ -e $f ]] || echo "$f"; done; }

ctx=$(context "$plugin")
check "no link is left relative" "" "$(relative "$ctx")"
check "every path the context links to exists" "" "$(missing "$ctx")"

# Copies whose rules are broken in one way each must have that reported.
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
copy() {
  rm -rf "$tmp/plugins"
  mkdir -p "$tmp/plugins"
  cp -R "$plugin" "$tmp/plugins/conventions"
  cp -R "$(dirname "$plugin")/principles" "$tmp/plugins/principles"
  printf '\n%s\n' "$1" >>"$tmp/plugins/conventions/rules/common.md"
}

copy '- [ない文書](../docs/missing.md)'
broken=$(context "$tmp/plugins/conventions")
check "a link to a missing document is reported" "$tmp/plugins/conventions/docs/missing.md" "$(missing "$broken")"

copy '- [書き換えられない](other.md)'
broken=$(context "$tmp/plugins/conventions")
check "a link not turned into a path is reported" "other.md" "$(relative "$broken")"

exit "$failed"
