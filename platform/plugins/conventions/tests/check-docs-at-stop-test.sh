#!/usr/bin/env bash
# Checks that the Stop hook runs check-docs only when the branch or the working tree changed Markdown,
# and holds the turn only while it fails.
set -euo pipefail

hook=$(cd "$(dirname "$0")/.." && pwd)/scripts/check-docs-at-stop
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
origin=$tmp/origin
repo=$tmp/repo
git init --quiet -b main "$origin"
g() { git -C "$1" -c user.name=t -c user.email=t@example.com "${@:2}"; }
echo '# r' >"$origin/README.md"
cp "$origin/README.md" "$origin/CLAUDE.md"
g "$origin" add README.md CLAUDE.md
g "$origin" commit --quiet -m v1
git clone --quiet "$origin" "$repo"
g "$repo" switch --quiet -c work

# run [stop_hook_active]: whether the hook holds the turn, and with check-docs' errors.
run() {
  local out
  out=$(jq -nc --arg cwd "$repo" --argjson active "${1:-false}" \
    '{session_id: "s", cwd: $cwd, hook_event_name: "Stop", stop_hook_active: $active}' | "$hook")
  if [[ -z $out ]]; then
    echo pass
  elif jq -e '.decision == "block" and (.reason | contains("broken link"))' <<<"$out" >/dev/null; then
    echo block
  else
    echo "unexpected: $out"
  fi
}

check "nothing changed" pass "$(run)"
echo '[x](missing.md)' >>"$repo/README.md"
check "a broken link in the working tree holds the turn" block "$(run)"
check "not again once the turn went on for a Stop hook" pass "$(run true)"
g "$repo" commit --quiet -am broken
check "a broken link committed on the branch holds the turn" block "$(run)"
g "$repo" reset --quiet --hard origin/main
echo '[x](missing.md)' >"$repo/new.md"
check "an untracked document is checked" block "$(run)"
rm "$repo/new.md"
echo 'x' >"$repo/notes.txt"
check "a change to no Markdown is not checked" pass "$(run)"
rm "$repo/notes.txt"

# Broken on the default branch, but left alone by this branch: the turn is not held.
echo '[x](missing.md)' >>"$origin/README.md"
g "$origin" commit --quiet -am broken
g "$repo" fetch --quiet
g "$repo" reset --quiet --hard origin/main
check "documents broken before the branch are not this turn's" pass "$(run)"
echo '# r' >"$repo/CLAUDE.md"
echo '[r](README.md)' >>"$repo/CLAUDE.md"
check "touching any document checks them all" block "$(run)"

exit "$failed"
