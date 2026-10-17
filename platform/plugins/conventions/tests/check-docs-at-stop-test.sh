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

# run [cwd]: whether the hook holds the turn, and with check-docs' errors.
run() {
  local out
  out=$(jq -nc --arg cwd "${1:-$repo}" \
    '{session_id: "s", cwd: $cwd, hook_event_name: "Stop", stop_hook_active: false}' | "$hook")
  if [[ -z $out ]]; then
    echo pass
  elif jq -e '.decision == "block" and (.reason | contains("broken link"))' <<<"$out" >/dev/null; then
    echo block
  else
    echo "unexpected: $out"
  fi
}

held=$(git -C "$repo" rev-parse --path-format=absolute --git-path conventions-check-docs-held)
forget() { rm -f "$held"; }

check "nothing changed" pass "$(run)"
echo '[x](missing.md)' >>"$repo/README.md"
check "a broken link in the working tree holds the turn" block "$(run)"
check "the same failure holds a turn only once" pass "$(run)"
echo '[y](missing-too.md)' >>"$repo/README.md"
check "another failure holds the turn again" block "$(run)"
forget
mkdir -p "$repo/sub"
check "from a subdirectory, the whole repository is looked at" block "$(run "$repo/sub")"
forget
g "$repo" commit --quiet -am broken
check "a broken link committed on the branch holds the turn" block "$(run)"
g "$repo" remote set-head origin -d
forget
check "without the default branch, committed changes are not looked at" pass "$(run)"
echo '[z](missing-3.md)' >>"$repo/README.md"
check "without the default branch, the working tree still is" block "$(run)"
g "$repo" remote set-head origin main
g "$repo" reset --quiet --hard origin/main
check "a branch back to the default passes" pass "$(run)"
check "passing forgets the held failure" no "$([[ -e $held ]] && echo yes || echo no)"
echo '[x](missing.md)' >>"$repo/README.md"
check "a failure held before is told again after a pass" block "$(run)"
forget
g "$repo" reset --quiet --hard origin/main
echo '[x](missing.md)' >"$repo/new.md"
check "an untracked document is checked" block "$(run)"
rm "$repo/new.md"

# Broken on the default branch, but left alone by this branch: the turn is not held.
echo '[x](missing.md)' >>"$origin/README.md"
echo 'x' >"$origin/notes.txt"
g "$origin" add notes.txt
g "$origin" commit --quiet -am broken
g "$repo" fetch --quiet
g "$repo" reset --quiet --hard origin/main
check "documents broken before the branch are not this turn's" pass "$(run)"
echo 'y' >"$repo/notes.txt"
check "a tracked file that is not Markdown is not checked" pass "$(run)"
echo 'y' >"$repo/other.txt"
check "an untracked file that is not Markdown is not checked" pass "$(run)"
g "$repo" reset --quiet --hard origin/main
rm "$repo/other.txt"
echo '# l' >"$origin/later.md"
g "$origin" add later.md
g "$origin" commit --quiet -m later
g "$repo" fetch --quiet
check "documents the default branch changed after the branch left are not this turn's" pass "$(run)"
echo '[r](README.md)' >>"$repo/CLAUDE.md"
check "touching any document checks them all" block "$(run)"

exit "$failed"
