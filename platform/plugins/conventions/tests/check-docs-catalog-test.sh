#!/usr/bin/env bash
# Checks that check-docs --catalog derives each document's attributes from where they are first held.
set -euo pipefail

bin=$(cd "$(dirname "$0")/.." && pwd)/bin/check-docs
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
repo=$tmp/repo
git init --quiet -b main "$repo"
g() { git -C "$repo" -c user.name=t -c user.email=t@example.com "$@"; }
mkdir -p "$repo/docs/guides" "$repo/docs/adr" "$repo/agents"
printf '# r\n\n[docs](docs/README.md)\n' >"$repo/README.md"
cp "$repo/README.md" "$repo/CLAUDE.md"
printf '# docs\n\n[a](guides/a.md)\n[d](adr/0001-d.md)\n[r](../agents/r.md)\n' >"$repo/docs/README.md"
printf -- '---\nstatus: accepted\ndate: 2026-01-01\n---\n\n# 0001. D\n' >"$repo/docs/adr/0001-d.md"
printf -- '---\nname: r\n---\n\nrole\n' >"$repo/agents/r.md"
g add -A
GIT_COMMITTER_DATE=2026-01-02T00:00:00Z g commit --quiet -m v1
printf '~~~\n# not a title\n~~~\n\n# A\n\n[b](../../README.md)\n[w](../README.md "在りか")\n[d](../adr/0001-d.md)\n' >"$repo/docs/guides/a.md"

row() { (cd "$repo" && "$bin" --catalog) | awk -F'\t' -v p="$1" '$1 == p'; }
check "a decision record has its kind, title, date and status" \
  "docs/adr/0001-d.md	判断	0001. D	2026-01-02	accepted	-	-" "$(row docs/adr/0001-d.md)"
check "an index relies on nothing" "docs/README.md	索引	docs	2026-01-02	-	-	-" "$(row docs/README.md)"
check "a guide relies on its links but not on those that show where or on decision records" \
  "docs/guides/a.md	手順	A	-	-	-	README.md" "$(row docs/guides/a.md)"
check "a role without a heading is titled by its name" "agents/r.md	役割ごとの指示	r	2026-01-02	-	-	-" "$(row agents/r.md)"
check "the catalog ends without a traceback when its reader stops" "" "$(cd "$repo" && { "$bin" --catalog | true; } 2>&1)"

exit "$failed"
