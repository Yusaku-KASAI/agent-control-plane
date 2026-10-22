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
mkdir -p "$repo/docs/guides" "$repo/docs/adr" "$repo/p/.claude-plugin" "$repo/p/agents" "$repo/.claude/agents" "$repo/rules" \
  "$repo/skills/s"
echo '{}' >"$repo/p/.claude-plugin/plugin.json"
printf '# r\n\n[docs](docs/README.md)\n' >"$repo/README.md"
cp "$repo/README.md" "$repo/CLAUDE.md"
printf '# docs\n\n[o](overview.md)\n[a](guides/a.md)\n[d](adr/0001-d.md)\n[r](../p/agents/r.md)\n[h](../.claude/agents/h.md)\n' >"$repo/docs/README.md"
# The backquotes are Markdown, not a command.
# shellcheck disable=SC2016
printf '# O\n\n<a id="map-d"></a>\n\n| 項目 | 親 | 置き場所 | 前提にする |\n| --- | --- | --- | --- |\n| d | | `./` | |\n' >"$repo/docs/overview.md"
printf -- '---\nstatus: accepted\ndate: 2026-01-01\n---\n\n# 0001. D\n\n[a](../guides/a.md)\n' >"$repo/docs/adr/0001-d.md"
printf -- '---\nname: r\n---\n\nrole\n\n[o](../../docs/overview.md)\n' >"$repo/p/agents/r.md"
printf -- '---\nname: h\n---\n\nA helper.\n' >"$repo/.claude/agents/h.md"
printf '# C\n' >"$repo/rules/c.md"
printf '# T\n' >"$repo/docs/guides/手順.md"
printf -- '---\nname: s\n---\n\n# S\n' >"$repo/skills/s/SKILL.md"
g add -A
GIT_COMMITTER_DATE=2026-01-02T00:00:00Z g commit --quiet -m v1
v1=$(git -C "$repo" rev-parse --short HEAD)
printf '~~~\n# not a title\n~~~\n\n# A\n\n[b](../../README.md)\n[w](../README.md "在りか")\n[d](../adr/0001-d.md)\n' >"$repo/docs/guides/a.md"

row() { (cd "$repo" && "$bin" --catalog) | awk -F'\t' -v p="$1" '$1 == p'; }
check "the header names the columns" "文書	区分	タイトル	版	更新日	状態	項目	前提	在りか" \
  "$(cd "$repo" && "$bin" --catalog | head -1)"
check "an entry relies on its links without a title" "README.md	入口	r	$v1	2026-01-02	-	-	docs/README.md	-" "$(row README.md)"
check "an index only shows where documents are" \
  "docs/README.md	索引	docs	$v1	2026-01-02	-	d	-	docs/overview.md,docs/guides/a.md,docs/adr/0001-d.md,p/agents/r.md,.claude/agents/h.md" \
  "$(row docs/README.md)"
check "an overview is in the item its map declares" "docs/overview.md	overview	O	$v1	2026-01-02	-	d	-	-" \
  "$(row docs/overview.md)"
check "a decision record has its status and relies on nothing" \
  "docs/adr/0001-d.md	判断	0001. D	$v1	2026-01-02	accepted	d	-	docs/guides/a.md" "$(row docs/adr/0001-d.md)"
check "a guide relies on its links but not on those that show where or on decision records" \
  "docs/guides/a.md	手順	A	-	-	-	d	README.md	docs/README.md,docs/adr/0001-d.md" "$(row docs/guides/a.md)"
check "a role without a heading is titled by its name, and what it relies on is not all told by links" \
  "p/agents/r.md	役割ごとの指示	r	$v1	2026-01-02	-	-	docs/overview.md,?	?" "$(row p/agents/r.md)"
check "a helper agent beside the roles is no role" ".claude/agents/h.md	その他	h	$v1	2026-01-02	-	-	-	-" \
  "$(row .claude/agents/h.md)"
check "common rules are told by their place" "rules/c.md	共通のルール	C	$v1	2026-01-02	-	-	-	-" "$(row rules/c.md)"
check "a skill is told by its place, and what it relies on is not all told by links" \
  "skills/s/SKILL.md	スキル	S	$v1	2026-01-02	-	-	?	?" "$(row skills/s/SKILL.md)"
check "a document named outside ASCII has its version and date" "docs/guides/手順.md	手順	T	$v1	2026-01-02	-	d	-	-" \
  "$(row docs/guides/手順.md)"
printf '# C2\n' >"$repo/rules/c.md"
printf '# T2\n' >"$repo/docs/guides/手順.md"
check "a document changed in the work tree has its version marked" "rules/c.md	共通のルール	C2	$v1+	2026-01-02	-	-	-	-" \
  "$(row rules/c.md)"
check "a document named outside ASCII changed in the work tree has its version marked" \
  "docs/guides/手順.md	手順	T2	$v1+	2026-01-02	-	d	-	-" "$(row docs/guides/手順.md)"
check "the catalog ends without a traceback when its reader stops" "" "$(cd "$repo" && { "$bin" --catalog | true; } 2>&1)"

exit "$failed"
