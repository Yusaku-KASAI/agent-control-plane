#!/usr/bin/env bash
# Exercises prctl's review tracking and hooks against a throwaway repository and a fake gh that
# serves the PR and its reviews from files.
set -euo pipefail

prctl=$(cd "$(dirname "$0")/.." && pwd)/bin/prctl
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

state=$tmp/gh
mkdir -p "$state" "$tmp/bin"
echo '[]' >"$state/reviews.json"
echo '{"check_runs": []}' >"$state/checks.json"
cat >"$tmp/bin/gh" <<'GH'
#!/usr/bin/env bash
set -euo pipefail
dir=$FAKE_GH
args=" $* "
case $args in
  *" pr list "*) [[ -f $dir/pr.json ]] && cat "$dir/pr.json"; exit 0 ;;
  *" -X POST "*"/reviews "*)
    [[ ! -f $dir/fail ]] || exit 1
    commit=$(sed -E 's/.* commit_id=([0-9a-f]+) .*/\1/' <<<"$args")
    jq --arg c "$commit" '. + [{state: "COMMENTED", commit_id: $c, body: "review"}]' "$dir/reviews.json" >"$dir/r.tmp"
    mv "$dir/r.tmp" "$dir/reviews.json"
    echo "https://example.invalid/review"
    ;;
  *"/reviews "*)
    [[ ! -f $dir/fail ]] || exit 1
    jq -r "${@: -1}" "$dir/reviews.json"
    ;;
  *"/check-runs"*)
    [[ ! -f $dir/fail-checks ]] || exit 1
    jq -r "${@: -1}" "$dir/checks.json"
    ;;
  *) echo "fake gh: unexpected: $*" >&2; exit 1 ;;
esac
GH
chmod +x "$tmp/bin/gh"
export PATH=$tmp/bin:$PATH FAKE_GH=$state

git init -q --bare "$tmp/origin.git"
git clone -q "$tmp/origin.git" "$tmp/work" 2>/dev/null
cd "$tmp/work"
git config user.name test && git config user.email test@example.invalid
commit() { echo "$1" >>file && git add file && git commit -q -m "$1" && git rev-parse HEAD; }
base=$(commit base)
git push -q origin HEAD:main
git checkout -q -b feature
c1=$(commit one)
c2=$(commit two)
git push -q -u origin feature

# set_pr <head>: the open PR of feature, its body describing <head>.
set_pr() {
  jq -n --arg h "$1" '{number: 1, url: "u", headRefOid: $h, baseRefName: "main", body: "x\n<!-- prctl: head=\($h) -->"}' \
    >"$state/pr.json"
}
stop_hook() {
  rm -f "$(git rev-parse --git-path prctl-settled)"
  jq -n --arg cwd "$PWD" '{cwd: $cwd, stop_hook_active: false}' | "$prctl" hook stop
}

failures=0
check() {
  local name=$1 got=$2 want=$3
  if [[ $got == *"$want"* ]]; then echo "ok   $name"; else
    echo "FAIL $name: want '$want' in: $got"
    failures=$((failures + 1))
  fi
}

set_pr "$c2"
check "unreviewed starts where the branch left the base" "$("$prctl" unreviewed)" "range: $base..$c2"
check "stop asks for a review" "$(stop_hook)" "not reviewed up to ${c2:0:12}"
check "an empty review is refused" "$("$prctl" review "$c2" - </dev/null 2>&1 || true)" "the findings are empty"

# A review of c1, recorded after c2 is pushed, covers c1 only.
"$prctl" review "$c1" - <<<"指摘なし" >/dev/null
check "a review covers the commit it names" "$("$prctl" unreviewed)" "range: $c1..$c2"
check "a commit before the last review is refused" "$("$prctl" review "$base" - <<<"x" 2>&1 || true)" "is not after the last reviewed"

"$prctl" review "$c2" - <<<"指摘なし" >/dev/null
check "reviewed up to the head" "$("$prctl" unreviewed)" "reviewed up to the head"
out=$(stop_hook)
check "stop lets the turn end" "[$out]" "[]"

# checks <status/conclusion>...: the head's check runs, named c0, c1, ...
checks() {
  local i=0 r
  for r in "$@"; do
    jq -n --arg n "c$i" --arg s "${r%/*}" --arg c "${r#*/}" \
      '{name: $n, status: $s, conclusion: (if $c == "" then null else $c end), details_url: "https://ci.invalid/\($n)"}'
    i=$((i + 1))
  done | jq -s '{check_runs: .}' >"$state/checks.json"
}
settled() { cat "$(git rev-parse --git-path prctl-settled)" 2>/dev/null || true; }

# The CI of the head, each case: check runs | the state prctl ci gives | whether stop stops | settled.
while IFS='|' read -r runs want stops settles; do
  # shellcheck disable=SC2086 # the runs are words
  checks $runs
  out=$(stop_hook)
  check "ci [$runs] is $want" "$("$prctl" ci | sed -n 2p)" "$want"
  if [[ $stops == yes ]]; then check "ci [$runs] stops the turn" "$out" "CI failed on ${c2:0:12}"
  else check "ci [$runs] lets the turn end" "[$out]" "[]"; fi
  if [[ $settles == yes ]]; then check "ci [$runs] settles the head" "$(settled)" "$c2"
  else check "ci [$runs] leaves the head unsettled" "[$(settled)]" "[]"; fi
done <<'CASES'
completed/success completed/neutral completed/skipped|passed|no|yes
completed/failure completed/success|failed|yes|no
completed/timed_out|failed|yes|no
completed/failure in_progress/|failed|yes|no
in_progress/ completed/success|pending|no|no
completed/action_required completed/success|action_required|no|no
completed/cancelled completed/success|incomplete|no|no
completed/stale|incomplete|no|no
CASES

checks completed/failure completed/success completed/timed_out
check "failures are listed with their URLs" "$(stop_hook)" "c0 https://ci.invalid/c0; c2 https://ci.invalid/c2."
check "the post-tool-use hook leaves CI out" \
  "[$(rm -f "$(git rev-parse --git-path prctl-notified)"; jq -n --arg cwd "$PWD" '{cwd: $cwd}' | "$prctl" hook post-tool-use)]" "[]"
checks
check "no check run is none" "$("$prctl" ci | sed -n 2p)" "none"
out=$(stop_hook)
check "no check run and no workflow settle the head" "$(settled)" "$c2"
touch "$state/fail-checks"
out=$(stop_hook)
check "checks that cannot be read are left out" "[$out]" "[]"
check "checks that cannot be read leave the head unsettled" "[$(settled)]" "[]"
rm "$state/fail-checks"
echo '{"check_runs": []}' >"$state/checks.json"

c3=$(commit three)
git push -q
set_pr "$c3"
check "a new push needs its own review" "$("$prctl" unreviewed)" "range: $c2..$c3"
# A review without a body (a lone line comment or a reply; empty or null) and a dismissed review
# do not count.
jq '. + [{state: "COMMENTED", commit_id: "'"$c3"'", body: ""}, {state: "COMMENTED", commit_id: "'"$c3"'", body: null},
  {state: "DISMISSED", commit_id: "'"$c3"'", body: "x"}]' \
  "$state/reviews.json" >"$state/r" && mv "$state/r" "$state/reviews.json"
check "reviews without a body and dismissed reviews do not count" "$("$prctl" unreviewed)" "range: $c2..$c3"
check "post-tool-use reminds of the review" \
  "$(jq -n --arg cwd "$PWD" '{cwd: $cwd}' | "$prctl" hook post-tool-use)" "not reviewed up to ${c3:0:12}"
other=$(git commit-tree -m outside "$(git rev-parse "HEAD^{tree}")")
check "a commit not in the PR is refused" "$("$prctl" review "$other" - <<<"x" 2>&1 || true)" "is not in the PR"

# Commits merged in from the base are not part of the range.
git checkout -q main
echo side >side && git add side && git commit -q -m main-side
git push -q origin main
git checkout -q feature
git merge -q --no-edit main
c5=$(git rev-parse HEAD)
git push -q
set_pr "$c5"
out=$("$prctl" unreviewed)
check "a merge of the base is pointed out" "$out" "the range merges the base"
[[ $out != *"main-side"* ]] || { echo "FAIL the base's commit is in the range"; failures=$((failures + 1)); }
check "the range keeps the branch's commits" "$out" "three"

# History rewritten past every review: the whole branch is unreviewed again.
git reset -q --hard "$base"
c4=$(commit rewritten)
git push -q -f
set_pr "$c4"
check "a rewrite unreviews the branch" "$("$prctl" unreviewed)" "range: $base..$c4"

# The reviews cannot be read: the hook stays silent and the commands fail.
touch "$state/fail"
out=$(stop_hook) && rc=0 || rc=$?
check "stop is silent when reviews cannot be read" "$out|$rc" "|0"
[[ -z $out ]] || { echo "FAIL stop printed: $out"; failures=$((failures + 1)); }
check "unreviewed fails when reviews cannot be read" "$("$prctl" unreviewed 2>&1 || true)" "could not read the reviews"
rm "$state/fail"

((failures == 0)) || { echo "$failures failed"; exit 1; }
