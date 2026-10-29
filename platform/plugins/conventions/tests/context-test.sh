#!/usr/bin/env bash
# Checks the context scripts/context injects: the rules link their sources relative to rules/, and
# every link to a file must reach the reader as an absolute path that exists. So must the paths of the
# last line, which tells where the documents are without links, and the output must name the hook's event.
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

# Each event the hooks run scripts/context for must reach the output by its name.
wired() {
  local plugin=$1 event command
  while IFS=$'\t' read -r event command; do
    echo "$event $(CLAUDE_PLUGIN_ROOT=$plugin bash -c "$command" | jq -r .hookSpecificOutput.hookEventName)"
  done < <(jq -r '.hooks | to_entries[] | .key as $e | .value[].hooks[].command | select(test("scripts/context"))
    | [$e, .] | @tsv' "$plugin/hooks/hooks.json")
}

# The paths the last line tells, one per line: the documents of this plugin, then those of principles.
told() { tail -n 1 <<<"$1" | sed -n 's/^文書の在りか：決まりは \(.*\)、原則とパターンは \(.*\)。$/\1\n\2/p'; }

ctx=$(context "$plugin")
check "no link is left relative" "" "$(relative "$ctx")"
check "every path the context links to exists" "" "$(missing "$ctx")"
check "the last line tells where the documents of conventions and principles are" \
  "$plugin/docs/README.md
$(dirname "$plugin")/principles/docs/README.md" "$(told "$ctx")"
check "every path the last line tells exists" "" "$(told "$ctx" | while read -r f; do [[ -f $f ]] || echo "$f"; done)"
check "the hooks run it at the start of sessions and subagents, and the output names the event" \
  "SessionStart SessionStart
SubagentStart SubagentStart" "$(wired "$plugin")"

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

copy ''
rm -rf "$tmp/plugins/principles"
broken=$(context "$tmp/plugins/conventions")
check "a copy without principles beside it names where to look" \
  "$tmp/plugins/conventions/docs/README.md
同じマーケットプレイスの plugins/principles/docs/README.md" "$(told "$broken")"

exit "$failed"
