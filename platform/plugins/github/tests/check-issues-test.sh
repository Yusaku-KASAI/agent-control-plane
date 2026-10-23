#!/usr/bin/env bash
# Exercises check-issues against a fake gh that serves the open issues, the pinned roadmap, the
# state of issues and their dependencies from files.
set -euo pipefail

cmd=$(cd "$(dirname "$0")/.." && pwd)/bin/check-issues
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

state=$tmp/gh
mkdir -p "$state" "$tmp/bin"
cat >"$tmp/bin/gh" <<'GH'
#!/usr/bin/env bash
set -euo pipefail
dir=$FAKE_GH
args=" $* "
jq_filter() { local prev=; for a in "$@"; do [[ $prev == --jq ]] && { echo "$a"; return; }; prev=$a; done; echo .; }
case $args in
  *" issue list -R o/r "*) [[ ! -f $dir/fail-list ]] || exit 1; cat "$dir/issues.json" ;;
  *" label list -R o/r "*) jq -r "$(jq_filter "$@")" "$dir/labels.json" ;;
  *" api graphql "*) jq -r "$(jq_filter "$@")" "$dir/pinned.json" ;;
  *" api repos/o/r/issues/"*"/dependencies/blocked_by "*)
    n=${args#* api repos/o/r/issues/}; n=${n%%/*}
    jq -r "$(jq_filter "$@")" "$dir/blocked-$n.json" 2>/dev/null || echo "" ;;
  *" api repos/o/r/issues/"*)
    n=${args#* api repos/o/r/issues/}; n=${n%% *}
    [[ ! -f $dir/gone-$n ]] || { echo "gh: Not Found (HTTP 404)" >&2; exit 1; }
    f=$dir/issue-$n.json; [[ -f $f ]] || f=$dir/open.json
    jq -r "$(jq_filter "$@")" "$f" ;;
  *) echo "unexpected: $*" >&2; exit 3 ;;
esac
GH
chmod +x "$tmp/bin/gh"
export PATH=$tmp/bin:$PATH FAKE_GH=$state
unset GITHUB_REPOSITORY

failures=0
check() {
  local name=$1 want=$2 got=$3
  if [[ $got == "$want" ]]; then echo "ok   $name"; else echo "FAIL $name: got '$got', want '$want'"; failures=$((failures + 1)); fi
}
issue() { jq -nc --argjson n "$1" --arg labels "$2" '{number: $n, title: "t", labels: ($labels | split(",") | map(select(. != "")) | map({name: .}))}'; }
roadmap() { jq -n --arg body "$1" '{data: {repository: {pinnedIssues: {nodes: [{issue: {number: 42, title: "ロードマップ", body: $body}}]}}}}' >"$state/pinned.json"; }
reset() {
  rm -f "$state"/*
  { issue 1 app,theme:ops; issue 42 ""; issue 9 theme; } | jq -s . >"$state/issues.json"
  roadmap $'## 今の焦点\n\n1. #1 をする\n\n## その後\n\n- #5 は閉じた'
  echo '{"state": "open"}' >"$state/open.json"
  echo '[{"name": "bug", "isDefault": true}, {"name": "accessibility", "isDefault": true}, {"name": "app", "isDefault": false}]' >"$state/labels.json"
  echo '{"state": "closed"}' >"$state/issue-5.json"
}
run() { "$cmd" o/r >"$tmp/out" 2>"$tmp/err"; echo $?; }

reset
check "a tidy repository passes" 0 "$(run)"
check "a closed issue outside 今の焦点 is not counted" 0 "$(grep -c '^NG' "$tmp/out" || true)"

reset
{ issue 1 app; issue 42 ""; } | jq -s . >"$state/issues.json"
check "an issue without a theme label fails" 1 "$(run)"
check "the issue and what it lacks are named" "NG   #1 theme:<名前> のラベルがない" "$(grep '^NG' "$tmp/out")"

reset
{ issue 1 theme:ops,canvas; issue 42 ""; } | jq -s . >"$state/issues.json"
check "canvas is not a label of where" "NG   #1 どこを変えるか のラベルがない" "$(run >/dev/null; grep '^NG' "$tmp/out")"

reset
echo '{"state": "closed"}' >"$state/issue-1.json"
check "a closed issue in 今の焦点 fails" 1 "$(run)"
check "the closed issue is named" "NG   the roadmap #42 names closed issues in 今の焦点: #1" "$(grep '^NG' "$tmp/out")"

reset
roadmap $'## 今の焦点\n\n1. other-repo#5 を通す'
check "an issue of another repository is not this one's" 0 "$(run)"

reset
jq -n '{data: {repository: {pinnedIssues: {nodes: []}}}}' >"$state/pinned.json"
{ issue 1 app,theme:ops; } | jq -s . >"$state/issues.json"
check "without a roadmap the labels still count" 0 "$(run)"
check "without a roadmap the focus is told as not checked" 1 "$(grep -c 'not checked' "$tmp/out")"
{ issue 1 app,theme:ops; issue 42 ""; } | jq -s . >"$state/issues.json"
check "without a roadmap, an issue titled so is held to the labels" 1 "$(run)"

reset
echo '[{"number": 7, "state": "closed"}, {"number": 8, "state": "open"}]' >"$state/blocked-1.json"
check "an issue blocked by a closed one fails" 1 "$(run)"
check "only the closed blockers are named" "NG   #1 is blocked by closed issues: #7" "$(grep '^NG' "$tmp/out")"

reset
echo '{"state": "closed", "pull_request": {}}' >"$state/issue-1.json"
check "a pull request in 今の焦点 fails as no issue" 1 "$(run)"
check "the pull request is named as no issue, not as a closed one" \
  "NG   the roadmap #42 names in 今の焦点 what is no issue here (a pull request or a missing number): #1" "$(grep '^NG' "$tmp/out")"

reset
roadmap $'## 焦点\n\n1. #1 をする'
check "a roadmap without the section 今の焦点 fails" "NG   the roadmap #42 has no section ## 今の焦点" "$(run >/dev/null; grep '^NG' "$tmp/out")"
check "a roadmap without the section 今の焦点 is not also told as fine" 0 "$(grep -c 'names no closed' "$tmp/out" || true)"

reset
{ issue 1 theme:ops,accessibility; issue 42 ""; } | jq -s . >"$state/issues.json"
check "a default label of GitHub is not a label of where" "NG   #1 どこを変えるか のラベルがない" "$(run >/dev/null; grep '^NG' "$tmp/out")"

reset
touch "$state/gone-1"
echo '[{"number": 7, "state": "closed"}]' >"$state/blocked-1.json"
check "a number in 今の焦点 that is no issue here fails" 1 "$(run)"
check "the number is named and the dependencies are still checked" \
  "NG   the roadmap #42 names in 今の焦点 what is no issue here (a pull request or a missing number): #1
NG   #1 is blocked by closed issues: #7" "$(grep '^NG' "$tmp/out")"

reset
touch "$state/fail-list"
check "issues that cannot be read stop with 2" 2 "$(run)"

exit "$failures"
