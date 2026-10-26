#!/usr/bin/env bash
# Checks that the context scripts/context injects links to paths that exist: the rules link their
# sources relative to rules/, and scripts/context turns those links into absolute paths.
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

# The context a plugin at $1 injects; stops the test when scripts/context fails.
context() {
  local out
  if ! out=$("$1/scripts/context" SessionStart | jq -r .hookSpecificOutput.additionalContext); then
    echo "FAIL scripts/context of $1 failed"
    exit 1
  fi
  printf '%s\n' "$out"
}

# The absolute paths a context links to, one per line.
paths() { grep -o '](/[^)#"]*' <<<"$1" | cut -c3- | sed 's/ *$//'; }

# Of those paths, the ones that do not exist.
missing() { paths "$1" | while read -r f; do [[ -e $f ]] || echo "$f"; done; }

ctx=$(context "$plugin")
check "every path the context links to exists" "" "$(missing "$ctx")"
# The rules link to the documentation system relative to rules/; it shows only once turned into a path.
check "a link of the rules is turned into a path" "yes" \
  "$(paths "$ctx" | grep -qxF "$plugin/docs/architecture/documentation.md" && echo yes || echo no)"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/plugins"
cp -R "$plugin" "$tmp/plugins/conventions"
cp -R "$(dirname "$plugin")/principles" "$tmp/plugins/principles"
copy=$tmp/plugins/conventions

# A copy whose rules link to a missing document must have that path reported.
printf '\n- [ない文書](../docs/missing.md)\n' >>"$copy/rules/common.md"
check "a link to a missing document is reported" "$copy/docs/missing.md" "$(missing "$(context "$copy")")"

# A copy that injects the rules without turning their links into paths must fail the check above.
# The sed script is the text to put in place, not an expansion.
# shellcheck disable=SC2016
sed -i.orig 's/^ctx="\$(sed .*/ctx="$(cat "$root"\/rules\/*.md)/' "$copy/scripts/context"
check "a context that does not turn the links into paths is caught" "no" \
  "$(paths "$(context "$copy")" | grep -qxF "$copy/docs/architecture/documentation.md" && echo yes || echo no)"

exit "$failed"
