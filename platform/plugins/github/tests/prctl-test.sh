#!/usr/bin/env bash
# Exercises prctl's review tracking and hooks against a throwaway repository and a fake gh that
# serves the PR and its reviews from files.
set -euo pipefail

prctl=$(cd "$(dirname "$0")/.." && pwd)/bin/prctl
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
# Where prctl keeps its worktrees and locks.
export TMPDIR=$tmp

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
  *" pr view 1 "*) cat "$dir/pr.json" ;;
  *" repo view "*) echo main ;;
  *" -X POST "*"/reviews "*)
    [[ ! -f $dir/fail ]] || exit 1
    commit="" body=""
    for a in "$@"; do
      case $a in commit_id=*) commit=${a#commit_id=} ;; body=*) body=${a#body=} ;; esac
    done
    printf '%s' "$body" >"$dir/last-body"
    jq --arg c "$commit" --arg b "$body" '. + [{state: "COMMENTED", commit_id: $c, body: $b}]' "$dir/reviews.json" >"$dir/r.tmp"
    mv "$dir/r.tmp" "$dir/reviews.json"
    echo "https://example.invalid/review"
    ;;
  *"/reviews "*)
    [[ ! -f $dir/fail ]] || exit 1
    jq -r "${@: -1}" "$dir/reviews.json"
    ;;
  *"/check-runs"*)
    [[ ! -f $dir/fail-checks ]] || exit 1
    # Something to happen while prctl waits, once: on-checks is run before answering.
    if [[ -f $dir/on-checks ]]; then bash "$dir/on-checks"; rm "$dir/on-checks"; fi
    if [[ -f $dir/fail-checks-once ]]; then rm "$dir/fail-checks-once"; exit 1; fi
    # A sequence of answers, one per call, the last one repeated: seq/1.json, seq/2.json, ...
    if [[ -d $dir/seq ]]; then
      next=$(find "$dir/seq" -name '*.json' | sort -V | head -n 1)
      cp "$next" "$dir/checks.json"
      [[ $(find "$dir/seq" -name '*.json' | wc -l) -eq 1 ]] || rm "$next"
    fi
    jq -r "${@: -1}" "$dir/checks.json"
    ;;
  *"/issues/1/comments "*)
    for a in "$@"; do [[ $a != body=* ]] || printf '%s' "${a#body=}" >"$dir/last-comment"; done
    echo "https://example.invalid/comment"
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

# Reviews in the form prctl takes: nothing found, looked at from every perspective, as a diff or whole.
all='["別の形", "依頼との一致", "正しさ", "設計", "セキュリティ", "文書とコメント", "検査"]'
nothing=$(jq -nc --argjson p "$all" '{scope: "since", perspectives: $p, findings: []}')
whole=$(jq -nc --argjson p "$all" '{scope: "pr", perspectives: $p, findings: []}')

failures=0
check() {
  local name=$1 got=$2 want=$3
  if [[ $got == *"$want"* ]]; then echo "ok   $name"; else
    echo "FAIL $name: want '$want' in: $got"
    failures=$((failures + 1))
  fi
}
# lacks <name> <got> <wanted> <unwanted>: <got> has <wanted> (so that it is not empty) but not <unwanted>.
lacks() {
  local name=$1 got=$2 wanted=$3 unwanted=$4
  if [[ $got == *"$wanted"* && $got != *"$unwanted"* ]]; then echo "ok   $name"; else
    echo "FAIL $name: want '$wanted' and not '$unwanted' in: $got"
    failures=$((failures + 1))
  fi
}

set_pr "$c2"
check "unreviewed starts where the branch left the base" "$("$prctl" unreviewed)" "range: $base..$c2"
out=$(stop_hook)
check "stop asks for a review" "$out" "not reviewed up to ${c2:0:12}"
lacks "stop does not point to minor fixes before any review" "$out" "not reviewed up to" "prctl review --minor"
lacks "unreviewed does not point to minor fixes before any review" "$("$prctl" unreviewed)" "range:" "prctl review --minor"
check "an empty review is refused" "$("$prctl" review "$c2" - </dev/null 2>&1 || true)" "the review is not JSON"
check "a review not in the form is refused" \
  "$("$prctl" review "$c2" - <<<'{"scope": "since", "perspectives": ["正しさ"], "findings": [{"severity": "重い"}]}' 2>&1 || true)" \
  "skipped says why these were not looked at: 別の形"
check "minor fixes before any review are refused" "$("$prctl" review --minor "$c2" - <<<"x" 2>&1 || true)" "has no review yet"

# A review of c1, recorded after c2 is pushed, covers c1 only.
check "the first review since the last is refused" "$("$prctl" review "$c1" - <<<"$nothing" 2>&1 || true)" "review the whole PR"
"$prctl" review "$c1" - <<<"$whole" >/dev/null
check "a review covers the commit it names" "$("$prctl" unreviewed)" "range: $c1..$c2"
check "a commit before the last review is refused" "$("$prctl" review "$base" - <<<"$nothing" 2>&1 || true)" "is not after the last reviewed"

"$prctl" review "$c2" - <<<"$nothing" >/dev/null
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

# prctl ci --wait follows the CI until it has finished.
export PRCTL_CI_POLL=0
# sequence <runs>...: the answers to come, one call each.
sequence() {
  local i=1 runs
  rm -rf "$state/seq" && mkdir "$state/seq"
  for runs in "$@"; do
    # shellcheck disable=SC2086 # the runs are words
    checks $runs
    mv "$state/checks.json" "$state/seq/$i.json"
    i=$((i + 1))
  done
}
sequence "in_progress/" "in_progress/ completed/success" "completed/success completed/success"
check "wait follows the CI to its end" "$("$prctl" ci --wait | sed -n 2p)" "passed"
sequence "in_progress/" "completed/failure"
check "wait ends at a failure" "$("$prctl" ci --wait | sed -n 2p)" "failed"
sequence "in_progress/" "completed/success"
touch "$state/fail-checks-once"
check "wait outlasts a passing read error" "$("$prctl" ci --wait 2>&1 | sed -n 2p)" "passed"
sequence "in_progress/"
touch "$state/fail-checks"
check "wait gives up after three read errors in a row" "$("$prctl" ci --wait 2>&1 || true)" "could not read the checks"
rm "$state/fail-checks"
check "wait gives up at the timeout" "$(PRCTL_CI_TIMEOUT=0 "$prctl" ci --wait | sed -n 2p)" "timeout"

# The wait follows this clone's ref of the branch, also when it is pushed again while waiting.
next=$(git commit-tree -p "$c2" -m next "$(git rev-parse "$c2^{tree}")")
echo "git update-ref refs/remotes/origin/feature $next" >"$state/on-checks"
sequence "in_progress/" "completed/success"
out=$("$prctl" ci --wait)
check "wait follows a newer push" "$(head -n 1 <<<"$out")" "head: $next"
check "wait gives the newer push's CI" "$(sed -n 2p <<<"$out")" "passed"
set_pr "$c1"
check "wait starts from this clone's push when GitHub lags" "$("$prctl" ci --wait | head -n 1)" "head: $next"
git update-ref refs/remotes/origin/feature "$c2"
set_pr "$c2"
echo "git update-ref refs/remotes/origin/feature $c1" >"$state/on-checks"
sequence "in_progress/" "completed/success"
check "wait follows a push back to an older commit" "$("$prctl" ci --wait | head -n 1)" "head: $c1"
git update-ref refs/remotes/origin/feature "$c2"
# Also a push just as the wait has come to an end supersedes it.
echo "git update-ref refs/remotes/origin/feature $next" >"$state/on-checks"
sequence "completed/failure" "completed/success"
out=$("$prctl" ci --wait)
check "wait follows a push as it ends" "$(head -n 1 <<<"$out")" "head: $next"
check "wait gives the CI of the push as it ends" "$(sed -n 2p <<<"$out")" "passed"
git update-ref refs/remotes/origin/feature "$c2"
# Without this clone's ref of the branch, the PR head is waited for.
git update-ref -d refs/remotes/origin/feature
set_pr "$c1"
check "wait takes the PR head without a ref of the branch" "$("$prctl" ci --wait | head -n 1)" "head: $c1"
git update-ref refs/remotes/origin/feature "$c2"
set_pr "$c2"

# A wait outlives an edit of prctl in place (bash reads a script as it goes).
cp "$prctl" "$tmp/prctl-copy"
sequence "in_progress/"
PRCTL_CI_POLL=1 "$tmp/prctl-copy" ci --wait >"$tmp/edited.out" 2>&1 &
waiter=$!
sleep 2
script=$(cat "$tmp/prctl-copy")
# Longer than the last line, so that what is left at the old place does not parse.
printf '# ----------------------------------------------------------------\n%s\n' "$script" >"$tmp/prctl-copy"
sequence "completed/success"
rc=0 && wait "$waiter" || rc=$?
check "a wait outlives an edit of prctl in place" "|$rc|$(cat "$tmp/edited.out")" "|0|head: $c2"$'\n'"passed"

# One wait per branch: another ends at once, naming the one that waits. Without flock (not in the
# devcontainer), every wait goes ahead.
if command -v flock >/dev/null; then
  hold() { exec 9>>"$tmp/prctl-work-ci-wait-$1"; flock -n 9; echo 4242 >"$tmp/prctl-work-ci-wait-$1"; }
  hold feature
  sequence "in_progress/"
  check "a second wait ends at once" "$(PRCTL_CI_TIMEOUT=60 "$prctl" ci --wait)" "already waiting for the CI of feature (pid 4242)"
  exec 9>&-
  sequence "completed/success"
  check "the wait is free again once the first has ended" "$("$prctl" ci --wait | sed -n 2p)" "passed"
  # Branch names are encoded into the lock's name, so that feat/x and feat-x do not share one.
  git checkout -q -b feat/x && git push -q -u origin feat/x 2>/dev/null
  hold feat-x
  check "a lock of another branch does not stop the wait" "$("$prctl" ci --wait | sed -n 2p)" "passed"
  exec 9>&-
  hold "feat%2Fx"
  check "the lock of a branch with a slash is its own" "$(PRCTL_CI_TIMEOUT=60 "$prctl" ci --wait)" "already waiting"
  exec 9>&-
  git checkout -q feature
fi
rm -rf "$state/seq"

echo '{"check_runs": []}' >"$state/checks.json"

c3=$(commit three)
git push -q
set_pr "$c3"
out=$("$prctl" unreviewed)
check "a new push needs its own review" "$out" "range: $c2..$c3"
check "unreviewed points to minor fixes after a review" "$out" "prctl review --minor $c3"
# A review without a body (a lone line comment or a reply; empty or null) and a dismissed review
# do not count.
jq '. + [{state: "COMMENTED", commit_id: "'"$c3"'", body: ""}, {state: "COMMENTED", commit_id: "'"$c3"'", body: null},
  {state: "DISMISSED", commit_id: "'"$c3"'", body: "x"}]' \
  "$state/reviews.json" >"$state/r" && mv "$state/r" "$state/reviews.json"
check "reviews without a body and dismissed reviews do not count" "$("$prctl" unreviewed)" "range: $c2..$c3"
out=$(jq -n --arg cwd "$PWD" '{cwd: $cwd}' | "$prctl" hook post-tool-use)
check "post-tool-use reminds of the review" "$out" "not reviewed up to ${c3:0:12}"
check "post-tool-use points to minor fixes after a review" "$out" "prctl review --minor"
other=$(git commit-tree -m outside "$(git rev-parse "HEAD^{tree}")")
check "a commit not in the PR is refused" "$("$prctl" review "$other" - <<<"$nothing" 2>&1 || true)" "is not in the PR"

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
check "minor fixes after a rewrite are refused as such" "$("$prctl" review --minor "$c4" - <<<"x" 2>&1 || true)" \
  "none of the PR's reviews is in the history of the head"
lacks "unreviewed does not point to minor fixes after a rewrite" "$("$prctl" unreviewed)" "range:" "prctl review --minor"
lacks "stop does not point to minor fixes after a rewrite" "$(stop_hook)" "not reviewed up to" "prctl review --minor"

# The reviews cannot be read: the hook stays silent and the commands fail.
touch "$state/fail"
out=$(stop_hook) && rc=0 || rc=$?
check "stop is silent when reviews cannot be read" "$out|$rc" "|0"
[[ -z $out ]] || { echo "FAIL stop printed: $out"; failures=$((failures + 1)); }
check "unreviewed fails when reviews cannot be read" "$("$prctl" unreviewed 2>&1 || true)" "could not read the reviews"
rm "$state/fail"

# With a workflow in the head, no check run yet is not settled, wherever prctl runs from.
mkdir -p .github/workflows sub
echo "on: pull_request" >.github/workflows/ci.yml
echo x >sub/file
git add .github sub && git commit -q -m workflow && git push -q
c6=$(git rev-parse HEAD)
set_pr "$c6"
"$prctl" review "$c6" - <<<"$whole" >/dev/null
checks
rm -f "$(git rev-parse --git-path prctl-settled)"
out=$(jq -n --arg cwd "$PWD/sub" '{cwd: $cwd, stop_hook_active: false}' | "$prctl" hook stop)
check "no check run yet with a workflow leaves the head unsettled" "[$(settled)]" "[]"
check "wait reports when no check run appears" "$(PRCTL_CI_APPEAR=0 "$prctl" ci --wait | sed -n 2p)" "no check run appeared"

# A reviewer reads another's PR in a worktree of its own and reviews it whole, whatever was
# reviewed before.
c7=$(commit seven)
git push -q
git push -q origin "$c7:refs/pull/1/head"
set_pr "$c7"
check "a new commit is not reviewed yet" "$("$prctl" unreviewed)" "range: $c6..$c7"
git checkout -q main
before=$(git symbolic-ref HEAD; git rev-parse HEAD)
out=$("$prctl" worktree pr 1)
wt=$(sed -n 's/^path: //p' <<<"$out")
check "the PR is checked out in a worktree" "$(git -C "$wt" rev-parse HEAD)" "$c7"
check "the worktree leaves the work tree's HEAD" "$(git symbolic-ref HEAD; git rev-parse HEAD)" "$before"
[[ -z $(git -C "$wt" symbolic-ref -q HEAD) ]] || { echo "FAIL the worktree is on a branch"; failures=$((failures + 1)); }
from=$(git merge-base origin/main "$c7")
check "the range of another's PR is the whole PR" "$out" "range: $from..$c7"
check "a review of another's PR is recorded" \
  "$(cd "$wt" || exit; "$prctl" review --pr 1 "$c7" - <<<"$whole")" "https://example.invalid/review"
check "a review of another's PR covers the whole PR" "$(cat "$state/last-body")" "（${from:0:12}...${c7:0:12}、PR 全体）"
check "a review of another's PR counts for the author" "$("$prctl" unreviewed)" "reviewed up to the head"
check "a commit outside another's PR is refused" \
  "$(cd "$wt" || exit; "$prctl" review --pr 1 "$other" - <<<"$whole" 2>&1 || true)" "is not in the PR"
out=$(cd "$wt" || exit; "$prctl" worktree pr 1)
check "the worktree is made again from inside it" "$(git -C "$wt" rev-parse HEAD)" "$c7"
(cd "$wt" || exit; "$prctl" worktree remove pr 1)
[[ ! -e $wt ]] || { echo "FAIL the worktree is left after a remove from inside it"; failures=$((failures + 1)); }
check "removing the worktree drops its ref" "[$(git for-each-ref refs/prctl)]" "[]"
out=$("$prctl" worktree default)
wt=$(sed -n 's/^path: //p' <<<"$out")
check "the default branch is checked out in a worktree" "$(git -C "$wt" rev-parse HEAD)" "$(git rev-parse origin/main)"
"$prctl" worktree remove default
[[ ! -e $wt ]] || { echo "FAIL the default worktree is left"; failures=$((failures + 1)); }
mkdir -p "$wt"
check "a stranger at the worktree's path is not removed" "$("$prctl" worktree default 2>&1 || true)" "is not a worktree of this repository"
rmdir "$wt"
check "a stray argument is refused" "$("$prctl" worktree default 5 2>&1 || true)" "usage:"
git checkout -q feature

# Fixes that change no meaning are recorded as left unreviewed, and count as reviewed.
c8=$(commit eight)
git push -q
set_pr "$c8"
check "minor fixes are recorded" "$("$prctl" review --minor "$c8" - <<<"細部の直しだけ")" "https://example.invalid/review"
check "minor fixes are titled as left unreviewed" "$(cat "$state/last-body")" "## レビューしない直し（${c7:0:12}...${c8:0:12}）"
check "minor fixes count as reviewed" "$("$prctl" unreviewed)" "reviewed up to the head"
check "an empty note is refused" "$("$prctl" review --minor "$c8" - </dev/null 2>&1 || true)" "the note is empty"
c9=$(commit nine)
git push -q
set_pr "$c9"
lacks "unreviewed does not point to minor fixes after minor fixes" "$("$prctl" unreviewed)" "range: $c8..$c9" "prctl review --minor"
lacks "stop does not point to minor fixes after minor fixes" "$(stop_hook)" "not reviewed up to" "prctl review --minor"
check "minor fixes after minor fixes are refused" "$("$prctl" review --minor "$c9" - <<<"x" 2>&1 || true)" "is itself of fixes left unreviewed"
touch "$state/fail"
check "minor fixes are refused when the reviews cannot be read" "$("$prctl" review --minor "$c9" - <<<"x" 2>&1 || true)" "could not read the reviews"
rm "$state/fail"
"$prctl" review "$c9" - <<<"$nothing" >/dev/null
c10=$(commit ten)
git push -q
set_pr "$c10"
check "minor fixes after a review that follows minor fixes are recorded" "$("$prctl" review --minor "$c10" - <<<"x")" "https://example.invalid/review"
# A review among records at the same commit, wherever it comes: the range follows a review.
jq --arg c "$c10" '. + [{state: "COMMENTED", commit_id: $c, body: "別の人のレビュー"}, {state: "COMMENTED", commit_id: $c, body: "## レビューしない直し（x）"}]' \
  "$state/reviews.json" >"$state/r" && mv "$state/r" "$state/reviews.json"
c11=$(commit eleven)
git push -q
set_pr "$c11"
check "a review beside a record lets minor fixes follow" "$("$prctl" unreviewed)" "prctl review --minor $c11"
check "a stray argument to minor is refused" "$("$prctl" review --minor "$c10" 2>&1 || true)" "usage:"

# The form: a review is posted as a table, details and a hidden copy; answers must cover it.
c12=$(commit twelve)
git push -q
set_pr "$c12"
found=$(jq -nc --argjson p "$all" '{scope: "since", perspectives: ["正しさ", "文書とコメント"],
  skipped: ($p - ["正しさ", "文書とコメント"] | map({perspective: ., reason: "関わらない"})),
  findings: [{severity: "提案", perspective: "正しさ", location: "bin/prctl:1", summary: "a | b", basis: "状態を軸で洗い出す",
    problem: "p", fix: "f", verified: "動かした"}, {severity: "細部", perspective: "文書とコメント", location: "README.md",
    summary: "言い回し", basis: "書き方", problem: "p", fix: "f", verified: "読んだ", notes: "n"}]}')
"$prctl" review "$c12" - <<<"$found" >/dev/null
body=$(cat "$state/last-body")
check "a review is titled with its range" "$body" "## 差分のレビュー（"
check "a review since the last is titled as such" "$body" "...${c12:0:12}、前回から）"
check "a review names the perspectives looked at and left out" "$body" "見た観点：正しさ、文書とコメント。外した観点：別の形（関わらない）、依頼との一致（関わらない）、設計"
check "a review has a table of its findings" "$body" "| 1 | 提案 | 正しさ | bin/prctl:1 | a \\| b | 状態を軸で洗い出す |"
check "a review has the details below the table" "$body" "- **補足**：n"
check "a review carries a hidden copy of itself" "$(sed -n 's/.*<!-- prctl-review: \(.*\) -->.*/\1/p' <<<"$body" | base64 -d | jq -r .commit)" "$c12"
check "a review of another's PR since the last is refused" \
  "$("$prctl" review --pr 1 "$c12" - <<<"$nothing" 2>&1 || true)" "a review of another's PR is of scope \"pr\""
check "answers must cover every finding" \
  "$("$prctl" answer "$c12" - <<<'[{"finding": 1, "action": "直した", "commit": "abc"}]' 2>&1 || true)" "findings not answered: 2"
check "a fix names its commit" \
  "$("$prctl" answer "$c12" - <<<'[{"finding": 1, "action": "直した"}, {"finding": 2, "action": "直さない", "reason": "r"}]' 2>&1 || true)" \
  "answer to 1: a fix names its commit"
"$prctl" answer "$c12" - <<<'[{"finding": 2, "action": "直さない", "reason": "意味が変わらない"}, {"finding": 1, "action": "Issue にした", "issue": 140}]' >/dev/null
comment=$(cat "$state/last-comment")
check "answers are posted as a table" "$comment" "| 1 | Issue にした（#140） |"
check "answers carry a hidden copy" "$(sed -n 's/.*<!-- prctl-answers: \(.*\) -->.*/\1/p' <<<"$comment" | base64 -d | jq -r .review)" "$c12"
check "answers to a review prctl did not record are refused" "$("$prctl" answer "$c11" - <<<'[]' 2>&1 || true)" "no review of"
# A review of the whole PR covers it from where it left its base, whatever was reviewed before.
"$prctl" review "$c12" - <<<"$whole" >/dev/null
check "a review of the whole PR covers it" "$(cat "$state/last-body")" "## 差分のレビュー（$(git merge-base origin/main "$c12" | cut -c1-12)...${c12:0:12}、PR 全体）"
check "a review of the whole PR looks at every perspective" \
  "$("$prctl" review "$c12" - <<<'{"scope": "pr", "perspectives": ["正しさ"], "findings": []}' 2>&1 || true)" "a review of the whole PR looks at every perspective"

# The form refuses what it cannot tell apart, and checks the answers carried in a review too.
check "a finding at a perspective not looked at is refused" "$("$prctl" review "$c12" - <<<"$(jq -c '.findings[0].perspective = "セキュリティ"' <<<"$found")" 2>&1 || true)" \
  "finding 1: perspective is one of those looked at"
check "a perspective both looked at and skipped is refused" \
  "$("$prctl" review "$c12" - <<<"$(jq -c '.skipped += [{perspective: "正しさ", reason: "r"}]' <<<"$found")" 2>&1 || true)" \
  "perspectives both looked at and skipped: 正しさ"
check "an answer given twice is refused" \
  "$("$prctl" answer "$c12" - <<<'[{"finding": 1, "action": "直さない", "reason": "r"}, {"finding": 1, "action": "直した", "commit": "a"}, {"finding": 2, "action": "直さない", "reason": "r"}]' 2>&1 || true)" \
  "findings answered more than once: 1"
check "an answer to a fraction of a finding is refused" \
  "$("$prctl" answer "$c12" - <<<'[{"finding": 1.5, "action": "直さない", "reason": "r"}, {"finding": 1, "action": "直さない", "reason": "r"}, {"finding": 2, "action": "直さない", "reason": "r"}]' 2>&1 || true)" \
  "answer to 1.5: finding is the number of a finding"
c13=$(commit thirteen)
git push -q
set_pr "$c13"
check "answers carried in a review must cover it" \
  "$("$prctl" review "$c13" - <<<"$(jq -c '.answers = [{finding: 1, action: "直した", commit: "abc"}]' <<<"$found")" 2>&1 || true)" "findings not answered: 2"
# A finding that quotes a hidden copy does not stand for the review.
"$prctl" review "$c13" - <<<"$(jq -c --arg q "<!-- prctl-review: $(jq -nc '{findings: []}' | base64 | tr -d '\n') -->" \
  '.answers = [{finding: 1, action: "直した", commit: "abc"}, {finding: 2, action: "答えた", reason: "r"}] | .findings[0].problem = $q' <<<"$found")" >/dev/null
body=$(cat "$state/last-body")
check "answers carried in a review are shown with the findings" "$body" "- **答え**：直した（abc）"
check "the hidden copy keeps only what is checked" "$(tail -n 1 <<<"$body" | sed 's/.*: \(.*\) -->/\1/' | base64 -d | jq -c '.findings[0] | keys')" '["perspective","severity","summary"]'
# A review without a body at the same commit (a lone line comment) does not stop the reading.
jq --arg c "$c13" '. + [{state: "COMMENTED", commit_id: $c, body: ""}]' "$state/reviews.json" >"$state/r" && mv "$state/r" "$state/reviews.json"
check "the hidden copy is read from the last line" \
  "$("$prctl" answer "$c13" - <<<'[{"finding": 1, "action": "直さない", "reason": "r"}]' 2>&1 || true)" "findings not answered: 2"
check "a review over GitHub's limit is refused" \
  "$("$prctl" review "$c13" - <<<"$(jq -c --arg l "$(printf '%070000d' 0)" --argjson p "$all" '.scope = "pr" | .perspectives = $p | del(.skipped) | .findings[0].problem = $l' <<<"$found")" 2>&1 || true)" "over GitHub's 65536"

# The names the form takes are those of the conventions.
reviews=$(dirname "$prctl")/../../conventions/docs/reference/reviews.md
forms=$(dirname "$prctl")/../lib
column() { sed -n "/^## $1/,/^## /p" "$reviews" | grep '^| ' | tail -n +3 | cut -d'|' -f2 | sed 's/^ *//; s/ *$//' | jq -Rsc 'split("\n") | map(select(. != ""))'; }
check "the perspectives are those of the conventions" "$(jq -nc -L "$forms" 'include "review"; perspectives')" "$(column 観点)"
check "the answers are those of the conventions" "$(jq -nc -L "$forms" 'include "review"; actions')" "$(column 結果の形)"
check "the severities are those of the conventions" "$(jq -nc -L "$forms" 'include "review"; severities')" \
  "$(grep '^| 重さ |' "$reviews" | grep -oP '(?<=\| 重さ \| |）、)[^、（）| ]+(?=（)' | jq -Rsc 'split("\n") | map(select(. != ""))')"

((failures == 0)) || { echo "$failures failed"; exit 1; }
