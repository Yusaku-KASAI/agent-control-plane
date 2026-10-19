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
echo '[]' >"$state/comments.json"
echo '{"check_runs": []}' >"$state/checks.json"
cat >"$tmp/bin/gh" <<'GH'
#!/usr/bin/env bash
set -euo pipefail
dir=$FAKE_GH
args=" $* "
case $args in
  *" pr list "*" --state all "*)
    # The branch's latest PR: merged-<branch> #5 merged, closed-<branch> #6 closed; fail-all-<branch>
    # when it cannot be read.
    head=$(sed -E 's/.* --head ([^ ]+) .*/\1/' <<<"$args")
    [[ ! -f $dir/fail-all-${head//\//-} ]] || exit 1
    if [[ -f $dir/merged-${head//\//-} ]]; then echo "5 MERGED"
    elif [[ -f $dir/closed-${head//\//-} ]]; then echo "6 CLOSED"; fi
    ;;
  *" pr list "*)
    # nopr-<branch>: that branch has no open PR; fail-prs-<branch>: its PR cannot be read; otherwise
    # every branch has the one in pr.json.
    head=$(sed -E 's/.* --head ([^ ]+) .*/\1/' <<<"$args")
    [[ ! -f $dir/fail-prs-${head//\//-} ]] || exit 1
    [[ -f $dir/nopr-${head//\//-} ]] && exit 0
    [[ -f $dir/pr.json ]] && cat "$dir/pr.json"; exit 0
    ;;
  *" --jq .node_id "*) echo N1 ;;
  *" api graphql "*) printf '%s\n' "$*" >"$dir/last-graphql"; echo "https://example.invalid/pull/1" ;;
  *" pr create "*) printf '%s\n' "$*" >"$dir/last-create"; echo "https://example.invalid/pull/2" ;;
  *" pr view 1 "*) cat "$dir/pr.json" ;;
  *" repo view "*) echo main ;;
  *" -X PATCH "*"/pulls/"*)
    for a in "$@"; do [[ $a != body=* ]] || printf '%s' "${a#body=}" >"$dir/last-edit"; done
    echo "https://example.invalid/pull/1"
    ;;
  *" -X POST "*"/reviews "*)
    [[ ! -f $dir/fail ]] || exit 1
    commit="" body=""
    for a in "$@"; do
      case $a in commit_id=*) commit=${a#commit_id=} ;; body=*) body=${a#body=} ;; esac
    done
    printf '%s' "$body" >"$dir/last-body"
    jq --arg c "$commit" --arg b "$body" '. + [{id: (length + 1), state: "COMMENTED", commit_id: $c, body: $b}]' "$dir/reviews.json" >"$dir/r.tmp"
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
      next=$(find "$dir/seq" -name '*.json' 2>/dev/null | sort -V | head -n 1)
      if [[ -n $next ]] && cp "$next" "$dir/checks.json" 2>/dev/null; then
        [[ $(find "$dir/seq" -name '*.json' 2>/dev/null | wc -l) -le 1 ]] || rm -f "$next"
      fi
    fi
    jq -r "${@: -1}" "$dir/checks.json"
    ;;
  *"/issues/"*"/comments "*" body="*)
    for a in "$@"; do [[ $a != body=* ]] || printf '%s' "${a#body=}" >"$dir/last-comment"; done
    jq --rawfile b "$dir/last-comment" '. + [{body: $b}]' "$dir/comments.json" >"$dir/c.tmp" && mv "$dir/c.tmp" "$dir/comments.json"
    echo "https://example.invalid/comment"
    ;;
  *"/issues/"*"/comments "*) jq -r "${@: -1}" "$dir/comments.json" ;;
  # Labels: the label made, then put on the PR (pr.json) or taken off it.
  *" repos/{owner}/{repo}/labels "*) printf '%s\n' "$*" >>"$dir/labels-made" ;;
  *" -X POST "*"/issues/"*"/labels "*)
    for a in "$@"; do
      [[ $a != labels\[\]=* ]] || jq --arg l "${a#labels[]=}" '.labels += [{name: $l}]' "$dir/pr.json" >"$dir/p.tmp"
    done
    mv "$dir/p.tmp" "$dir/pr.json"
    ;;
  *" -X DELETE "*"/issues/"*"/labels/"*) jq '.labels = []' "$dir/pr.json" >"$dir/p.tmp" && mv "$dir/p.tmp" "$dir/pr.json" ;;
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

# set_pr <head>: the open PR of feature, its body describing <head>, reviewed the way $way says: the
# thorough way, which takes reviews of either scope, unless the normal way is what is tried.
way=thorough
set_pr() {
  jq -n --arg h "$1" --arg w "$way" '{number: 1, url: "u", headRefOid: $h, baseRefName: "main", body: "x\n<!-- prctl: head=\($h) -->",
    labels: (if $w == "thorough" then [{name: "念入りのレビュー"}] else [] end)}' >"$state/pr.json"
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
check "a PR without any review may not end" "$("$prctl" status)" "end:       PR #1 may not end yet: no review of the whole PR"
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

"$prctl" review "$c2" - <<<"$whole" >/dev/null
check "reviewed up to the head" "$("$prctl" unreviewed)" "reviewed up to the head"
out=$(stop_hook)
check "stop lets the turn end" "[$out]" "[]"

# check_run <name> <id> <status/conclusion> <url>: one check run in the form of GitHub's (an id, and
# the app that made it).
check_run() {
  jq -n --arg n "$1" --argjson id "$2" --arg s "${3%/*}" --arg c "${3#*/}" --arg u "$4" \
    '{id: $id, name: $n, status: $s, conclusion: (if $c == "" then null else $c end),
      details_url: $u, app: {id: 15368, slug: "github-actions"}}'
}
# checks <status/conclusion>...: the head's check runs, named c0, c1, ...
checks() { checks_to "$state/checks.json" "$@"; }
# checks_to <file> <status/conclusion>...: the same, written to <file>.
checks_to() {
  local file=$1 i=0 r
  shift
  for r in "$@"; do
    check_run "c$i" "$((100 + i))" "$r" "https://ci.invalid/c$i"
    i=$((i + 1))
  done | jq -s '{check_runs: .}' >"$file"
}
# rerun <id> <status/conclusion>...: the check runs of another suite on the same head, with the
# same names as before (c0, c1, ...) and ids from <id>, added to those already there.
rerun() {
  local id=$1 i=0 r
  shift
  for r in "$@"; do
    check_run "c$i" "$((id + i))" "$r" "https://ci.invalid/c$i/$((id + i))"
    i=$((i + 1))
  done | jq -s --slurpfile old "$state/checks.json" '{check_runs: ($old[0].check_runs + .)}' >"$state/checks.tmp"
  mv "$state/checks.tmp" "$state/checks.json"
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
# A head can have more than one check suite; of the runs of the same check, the latest one counts.
checks completed/failure completed/failure
rerun 200 completed/success completed/success
check "a check that failed and then passed in a later suite has passed" "$("$prctl" ci | sed -n 2p)" "passed"
out=$(stop_hook)
check "a check that failed and then passed in a later suite lets the turn end" "[$out]" "[]"
checks completed/failure
rerun 200 queued/
check "a check that failed and is running again in a later suite is pending" "$("$prctl" ci | sed -n 2p)" "pending"
out=$(stop_hook)
check "a check that failed and is running again in a later suite lets the turn end" "[$out]" "[]"
checks completed/success completed/success
rerun 200 completed/failure completed/success
check "a check that passed and then failed in a later suite has failed" "$("$prctl" ci | sed -n 2,3p)" \
  "failed
c0 https://ci.invalid/c0/200"
checks completed/failure
rerun 50 completed/success
check "of the same check, the greatest id counts, not the order of the answer" "$("$prctl" ci | sed -n 2p)" "failed"
checks completed/failure
jq '.check_runs += [.check_runs[0] | .id = 300 | .app = {id: 1, slug: "another-app"} | .conclusion = "success"]' \
  "$state/checks.json" >"$state/checks.tmp" && mv "$state/checks.tmp" "$state/checks.json"
check "a check of the same name from another app does not hide a failure" "$("$prctl" ci | sed -n 2p)" "failed"

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
# The new answers replace the old by renames, as a wait may be asking meanwhile.
sequence() {
  local i=1 runs
  rm -rf "$state/seq.new" && mkdir "$state/seq.new"
  for runs in "$@"; do
    # Straight into the new sequence: checks.json is not touched, as a wait may be reading it.
    # shellcheck disable=SC2086 # the runs are words
    checks_to "$state/seq.new/$i.json" $runs
    i=$((i + 1))
  done
  if [[ -d $state/seq ]]; then mv "$state/seq" "$state/seq.old"; fi
  mv "$state/seq.new" "$state/seq"
  rm -rf "$state/seq.old"
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
# A bound, so that a wait that never gets its answer fails the check instead of stalling the tests.
PRCTL_CI_POLL=1 PRCTL_CI_TIMEOUT=60 "$tmp/prctl-copy" ci --wait >"$tmp/edited.out" 2>&1 &
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

# A PR body has its sections, in order, none empty; what is under them is free.
good=$'## 何を変えるか\nx\n\n## 当てはめた決まり\n- 仕組みで守る\n\n## どう確かめたか\ny\n\n## 確かめていないこと\nなし\n\nCloses #1'
check "a body without a section is refused" "$("$prctl" create t - <<<"${good/\#\# 当てはめた決まり/\#\# 決まり}" 2>&1 || true)" "no section ## 当てはめた決まり"
check "a body with an empty section is refused" "$("$prctl" update - <<<"$(sed '/^なし$/,$d' <<<"$good")" 2>&1 || true)" "the section ## 確かめていないこと is empty"
check "a body with the sections out of order is refused" \
  "$("$prctl" create t - <<<"$(sed 's/## 何を変えるか/## どう確かめたか2/; s/## どう確かめたか$/## 何を変えるか/; s/## どう確かめたか2/## どう確かめたか/' <<<"$good")" 2>&1 || true)" "not in the order"
check "a body in the form goes on" "$("$prctl" create t - <<<"$good" 2>&1 || true)" "already has an open PR"
# shellcheck disable=SC2001,SC2016 # line-wise edits of a sample body, with Markdown backquotes
check "an issue to close does not fill the last section" \
  "$("$prctl" create t - <<<"$(sed 's/^なし$//' <<<"$good")"$'\n\n🤖 Generated' 2>&1 || true)" "the section ## 確かめていないこと is empty"
# shellcheck disable=SC2001,SC2016 # a line-wise edit of a sample body, with Markdown backquotes
check "a heading in a code block is not a section" \
  "$("$prctl" create t - <<<"$(sed 's/^x$/```md\n## どう確かめたか\n```/' <<<"$good")" 2>&1 || true)" "already has an open PR"
check "a section twice is refused as such" "$("$prctl" create t - <<<"$good"$'\n## 何を変えるか\nz' 2>&1 || true)" "the section ## 何を変えるか appears 2 times"
# shellcheck disable=SC2001 # a line-wise edit of a sample body
check "spaces after a heading do not hide it" "$("$prctl" create t - <<<"$(sed 's/^## 何を変えるか$/## 何を変えるか  /' <<<"$good")" 2>&1 || true)" "already has an open PR"
check "other sections may come anywhere" "$("$prctl" create t - <<<"## 背景"$'\nb\n'"$good" 2>&1 || true)" "already has an open PR"
# The sections prctl asks for are those of the conventions.
writing=$(dirname "$prctl")/../../conventions/docs/reference/writing.md
# shellcheck disable=SC2016 # backquotes of Markdown
check "the body's sections are those of the conventions" \
  "$(sed -n 's/^readonly body_sections=(\(.*\))$/\1/p' "$prctl")" \
  "$(sed -n '/^## PR$/,/^## /p' "$writing" | grep -o '^  | `## [^`]*`' | sed 's/.*## //; s/`//' | paste -sd ' ' -)"

# A PR is stacked on another PR's branch only when built on it, and only on an open PR.
git checkout -q -b stacked
commit fourteen >/dev/null
git push -q -u origin stacked 2>/dev/null
touch "$state/nopr-stacked"
check "a PR stacked on its base's branch is opened into it" \
  "$("$prctl" create --base feature t - <<<"$good" >/dev/null && cat "$state/last-create")" "--base feature --head stacked"
check "a PR into the default branch is opened as before" \
  "$("$prctl" create t - <<<"$good" >/dev/null && cat "$state/last-create")" "--base main --head stacked"
git push -q origin "$c2:refs/heads/elsewhere" && git fetch -q origin
check "a PR not built on the base is refused" "$("$prctl" create --base elsewhere t - <<<"$good" 2>&1 || true)" "is not built on origin/elsewhere"
touch "$state/nopr-feature"
check "a PR on a branch without an open PR is refused" "$("$prctl" create --base feature t - <<<"$good" 2>&1 || true)" "has no open PR to stack on"
rm "$state/nopr-feature" "$state/nopr-stacked"
check "a stray argument to create is refused" "$("$prctl" create --base feature t 2>&1 || true)" "usage:"
touch "$state/nopr-stacked"
check "a PR on its own branch is refused" "$("$prctl" create --base stacked t - <<<"$good" 2>&1 || true)" "cannot be stacked on its own branch"
check "a PR on a branch not here is refused" "$("$prctl" create --base nosuch t - <<<"$good" 2>&1 || true)" "origin/nosuch is not here"
rm "$state/nopr-stacked"
# A stacked PR is told when the branch below moves on without it, and when that has no open PR any more.
git remote set-head origin main
jq -n --arg h "$(git rev-parse HEAD)" '{number: 2, url: "u", headRefOid: $h, baseRefName: "feature", body: "x\n<!-- prctl: head=\($h) -->"}' >"$state/pr.json"
lacks "a stacked PR on its base's head is told nothing" "$("$prctl" status)" "PR #2" "stacked on"
git push -q origin "$(git commit-tree -p "$(git rev-parse origin/feature)" -m below "$(git rev-parse "origin/feature^{tree}")"):refs/heads/feature" && git fetch -q origin
check "a stacked PR is told to take in its base" "$("$prctl" status)" "merge origin/feature in and push"
touch "$state/nopr-feature" "$state/merged-feature"
check "a stacked PR is told when its base's PR was merged" "$(stop_hook)" "whose PR #5 was merged"
rm "$state/merged-feature"
touch "$state/closed-feature"
check "a stacked PR is told when its base's PR was closed unmerged" "$(stop_hook)" "closed without being merged. Do not move"
rm "$state/closed-feature"
check "a base without any PR tells nothing" "$("$prctl" status)" "PR #2"
lacks "a base without any PR is not taken as closed" "$("$prctl" status)" "PR #2" "stacked on"
touch "$state/fail-all-feature" "$state/closed-feature"
lacks "a stacked PR is told nothing when its base's latest PR cannot be read" "$(stop_hook)" "not reviewed up to" "stacked on"
rm "$state/fail-all-feature" "$state/closed-feature" "$state/nopr-feature"
touch "$state/fail-prs-feature"
lacks "a stacked PR is told nothing when its base's PR cannot be read" "$(stop_hook)" "not reviewed up to" "stacked on"
rm "$state/fail-prs-feature"
check "a stacked PR whose head is not here is told nothing of its base" \
  "$(jq '.headRefOid = "0123456789abcdef0123456789abcdef01234567"' "$state/pr.json" >"$state/p" && mv "$state/p" "$state/pr.json"; "$prctl" status)" "PR #2"
lacks "a head not here is not taken as behind its base" "$("$prctl" status)" "PR #2" "merge origin/feature in"
jq --arg h "$(git rev-parse HEAD)" '.headRefOid = $h' "$state/pr.json" >"$state/p" && mv "$state/p" "$state/pr.json"
git push -q -f origin "$c12:refs/heads/feature" 2>/dev/null; git fetch -q origin
git checkout -q feature
git reset -q --hard origin/feature

# Before a PR ends, a review of the whole PR settles it: no finding that must be fixed, every
# finding answered, nothing but fixes of nitpicks after it; or it goes back after the cap.
c15=$(commit fifteen)
git push -q
set_pr "$c15"
proposal=$(jq -c --argjson p "$all" '{scope: "pr", perspectives: $p, findings: [.findings[0]]}' <<<"$found")
"$prctl" review "$c15" - <<<"$proposal" >/dev/null
check "a whole review with an unanswered finding stops the end" "$(stop_hook)" "not answered: 1"
check "status tells what the end needs" "$("$prctl" status)" "end:       PR #1 may not end yet"
"$prctl" answer "$c15" - <<<'[{"finding": 1, "action": "直さない", "reason": "r"}]' >/dev/null
out=$(stop_hook)
check "a whole review with every finding answered lets the PR end" "[$out]" "[]"
check "status tells the PR may end" "$("$prctl" status)" "end:       may end"
c16=$(commit sixteen)
git push -q
set_pr "$c16"
"$prctl" review "$c16" - <<<"$nothing" >/dev/null
check "a change after the whole review stops the end" "$(stop_hook)" "review the whole PR again"
must=$(jq -c '.findings[0].severity = "必須"' <<<"$proposal")
"$prctl" review "$c16" - <<<"$must" >/dev/null
check "a finding that must be fixed stops the end" "$(stop_hook)" "must be fixed: 1"
c17=$(commit seventeen)
git push -q
set_pr "$c17"
"$prctl" review "$c17" - <<<"$must" >/dev/null
rm -f "$(git rev-parse --git-path prctl-handback-1)"
# The cap counts the reviews of the whole PR after the place last remembered: here, from this head.
handback=$(git rev-parse --git-path prctl-handback-1)
count_from_here() { git rev-parse HEAD >"$handback"; }
check "the cap of the thorough way is ten reviews of the whole PR by default" "$(grep -o 'PRCTL_FINAL_CAP:-[0-9]*' "$prctl")" "PRCTL_FINAL_CAP:-10"
count_from_here
cap_stop() { rm -f "$(git rev-parse --git-path prctl-settled)"; jq -n --arg cwd "$PWD" '{cwd: $cwd, stop_hook_active: false}' | PRCTL_FINAL_CAP=2 "$prctl" hook stop; }
ca=$(commit cap-a) && git push -q && set_pr "$ca" && "$prctl" review "$ca" - <<<"$must" >/dev/null
lacks "below the cap, the PR is not handed back" "$(cap_stop)" "must be fixed" "hand it back"
cb=$(commit cap-b) && git push -q && set_pr "$cb" && "$prctl" review "$cb" - <<<"$must" >/dev/null
check "at the cap, the PR goes back to the requester" "$(cap_stop)" "hand it back to the requester"
cc=$(commit cap-c) && git push -q && set_pr "$cc" && "$prctl" review "$cc" - <<<"$must" >/dev/null
lacks "after going back, the cap counts afresh" "$(cap_stop)" "must be fixed" "hand it back"
jq '.isDraft = true' "$state/pr.json" >"$state/p" && mv "$state/p" "$state/pr.json"
out=$(stop_hook)
check "a draft PR is not held for the whole review" "[$out]" "[]"
# Fixes of nitpicks after a settling whole review keep it.
c18=$(commit eighteen)
git push -q
set_pr "$c18"
"$prctl" review "$c18" - <<<"$whole" >/dev/null
c19=$(commit nineteen)
git push -q
set_pr "$c19"
"$prctl" review --minor "$c19" - <<<"細部の直し" >/dev/null
out=$(stop_hook)
check "fixes of nitpicks after the whole review let the PR end" "[$out]" "[]"

# PRs are opened as drafts; a draft is not held for the whole review, and prctl ready takes it out of
# draft only once it may end.
git checkout -q stacked
touch "$state/nopr-stacked"
check "a PR is opened as a draft" "$("$prctl" create t - <<<"$good" >/dev/null && cat "$state/last-create")" "pr create --draft"
rm "$state/nopr-stacked"
git checkout -q feature
c20=$(commit twenty)
git push -q
set_pr "$c20"
# A body read back from GitHub carries its old marker; only the marker of the head is sent.
old=$(printf '%040d' 0)
"$prctl" update - <<<"$good"$'\n\n<!-- prctl: head='"$old"$' -->' >/dev/null
check "a body's old marker is dropped" "[$(grep -o 'prctl: head=[0-9a-f]*' "$state/last-edit" | tr '\n' ' ')]" \
  "[prctl: head=$(git rev-parse HEAD) ]"
"$prctl" review "$c20" - <<<"$must" >/dev/null
jq '.isDraft = true' "$state/pr.json" >"$state/p" && mv "$state/p" "$state/pr.json"
out=$(stop_hook)
check "a draft is not held at the end of a turn" "[$out]" "[]"
check "a draft is not remembered as settled" "[$(settled)]" "[]"
check "status tells a draft is checked when made ready" "$("$prctl" status)" "end:       a draft; prctl ready checks it"
rm -f "$(git rev-parse --git-path prctl-handback-1)"
check "a draft that may not end is not made ready" "$("$prctl" ready 2>&1 || true)" "prctl: PR #1 may not end yet"
check "prctl ready does not start the count afresh" "[$(cat "$(git rev-parse --git-path prctl-handback-1)" 2>/dev/null)]" "[]"
c21=$(commit twentyone)
git push -q
set_pr "$c21"
jq '.isDraft = true' "$state/pr.json" >"$state/p" && mv "$state/p" "$state/pr.json"
"$prctl" review "$c21" - <<<"$whole" >/dev/null
rm -f "$state/last-graphql"
rm -f "$(git rev-parse --git-path prctl-handback-1)"
check "a draft that may end is made ready" "$("$prctl" ready)" "https://example.invalid/pull/1"
check "being made ready counts afresh" "$(cat "$(git rev-parse --git-path prctl-handback-1)")" "$c21"
check "being made ready goes through GraphQL" "$(cat "$state/last-graphql")" "markPullRequestReadyForReview"
jq '.isDraft = false' "$state/pr.json" >"$state/p" && mv "$state/p" "$state/pr.json"
check "a PR out of draft is told so" "$("$prctl" ready)" "is not a draft"
# Another review of the same commit is no change, and answers count for the review they answer.
"$prctl" review "$c21" - <<<"$nothing" >/dev/null 2>&1 || true
out=$(stop_hook)
check "another review of the commit of the whole review is no change" "[$out]" "[]"
c22=$(commit twentytwo)
git push -q
set_pr "$c22"
"$prctl" review "$c22" - <<<"$(jq -c '.findings[0].severity = "提案"' <<<"$found")" >/dev/null
"$prctl" review "$c22" - <<<"$proposal" >/dev/null
"$prctl" answer "$c22" - <<<'[{"finding": 1, "action": "直さない", "reason": "r"}]' >/dev/null
out=$(stop_hook)
check "answers count for the latest review of the commit only" "[$out]" "[]"
c23=$(commit twentythree)
git push -q
set_pr "$c23"
"$prctl" review "$c23" - <<<"$found" >/dev/null
"$prctl" answer "$c23" - <<<'[{"finding": 1, "action": "直さない", "reason": "r"}, {"finding": 2, "action": "直さない", "reason": "r"}]' >/dev/null
"$prctl" review "$c23" - <<<"$proposal" >/dev/null
check "answers to another review of the commit do not answer the whole review" "$(stop_hook)" "not answered: 1"

rm -f "$state/last-graphql"
check "a PR is put back into draft" "$("$prctl" draft)" "https://example.invalid/pull/1"
check "being put back into draft goes through GraphQL" "$(cat "$state/last-graphql")" "convertPullRequestToDraft"
# On the way of a draft: prctl ready tells it goes back but does not count afresh, being asked again;
# the end of the turn, not holding the draft, remembers it; then the count goes on afresh.
cd1=$(commit draft-a) && git push -q && set_pr "$cd1" && "$prctl" review "$cd1" - <<<"$must" >/dev/null
count_from_here
cd2=$(commit draft-b) && git push -q && set_pr "$cd2" && "$prctl" review "$cd2" - <<<"$must" >/dev/null
jq '.isDraft = true' "$state/pr.json" >"$state/p" && mv "$state/p" "$state/pr.json"
PRCTL_FINAL_CAP=1 "$prctl" ready >/dev/null 2>&1 || true
check "prctl ready tells a draft past the cap goes back, asked again" "$(PRCTL_FINAL_CAP=1 "$prctl" ready 2>&1 || true)" "hand it back"
out=$(rm -f "$(git rev-parse --git-path prctl-settled)"; jq -n --arg cwd "$PWD" '{cwd: $cwd, stop_hook_active: false}' | PRCTL_FINAL_CAP=1 "$prctl" hook stop)
check "the end of a draft's turn does not hold it" "[$out]" "[]"
cd3=$(commit draft-c) && git push -q && set_pr "$cd3" && "$prctl" review "$cd3" - <<<"$must" >/dev/null
jq '.isDraft = true' "$state/pr.json" >"$state/p" && mv "$state/p" "$state/pr.json"
lacks "after the end of a draft's turn, the count goes on afresh" "$(PRCTL_FINAL_CAP=2 "$prctl" ready 2>&1 || true)" "must be fixed" "hand it back"
jq '.isDraft = false' "$state/pr.json" >"$state/p" && mv "$state/p" "$state/pr.json"

# Out of draft, the end is asked along with the rest; a push not recorded yet asks to be recorded
# first; a mark out of the history counts all; the count goes afresh once the PR may end.
jq '.isDraft = false' "$state/pr.json" >"$state/p" && mv "$state/p" "$state/pr.json"
s1=$(commit settle-a) && git push -q && set_pr "$s1" && "$prctl" review "$s1" - <<<"$whole" >/dev/null
rm -f "$handback"
out=$(stop_hook)
check "a PR that may end lets the turn end" "[$out]" "[]"
check "the count goes afresh once the PR may end" "$(cat "$handback")" "$s1"
s2=$(commit settle-b) && git push -q && set_pr "$s2"
out=$(stop_hook)
check "the stop for a push not reviewed also asks for the whole review" "$out" "not reviewed up to ${s2:0:12}"
check "the stop for a push not reviewed names the end too" "$out" "may not end yet"
check "status does not say may end with a push not reviewed" "$("$prctl" status)" "end:       PR #1 may not end yet"
"$prctl" review "$s2" - <<<"$must" >/dev/null
lacks "after the count went afresh, one more is below the cap" "$(cap_stop)" "must be fixed" "hand it back"
echo "$other" >"$handback"
check "a mark out of the history counts every review of the whole PR" "$(cap_stop)" "hand it back"

# A push not recorded yet is neither a change nor a reason to go back; a turn held for another
# reason does not count afresh; another review of a settled head is looked at again.
u1=$(commit unrec-a) && git push -q && set_pr "$u1"
nit=$(jq -c --argjson p "$all" '{scope: "pr", perspectives: $p, findings: [.findings[1]]}' <<<"$found")
"$prctl" review "$u1" - <<<"$nit" >/dev/null
"$prctl" answer "$u1" - <<<'[{"finding": 1, "action": "直した", "commit": "x"}]' >/dev/null
u2=$(commit unrec-b) && git push -q && set_pr "$u2"
# Count from an earlier review of the whole PR (s1), so that u1 is counted and a wrong write of u1
# would show.
git rev-parse "$s1" >"$handback"
out=$(rm -f "$(git rev-parse --git-path prctl-settled)"; jq -n --arg cwd "$PWD" '{cwd: $cwd, stop_hook_active: false}' | PRCTL_FINAL_CAP=1 "$prctl" hook stop)
lacks "a push not recorded is not a reason to go back" "$out" "not recorded yet" "hand it back"
check "a push not recorded does not count afresh" "$(cat "$handback")" "$s1"
"$prctl" review --minor "$u2" - <<<"細部の直し" >/dev/null
checks completed/failure
out=$(stop_hook)
check "a turn held for CI is held" "$out" "CI failed on"
check "a turn held for CI does not count afresh" "$(cat "$handback")" "$s1"
checks completed/success
out=$(stop_hook)
check "a PR that may end with CI passing settles" "[$out]" "[]"
"$prctl" review "$u2" - <<<"$must" >/dev/null
out=$(jq -n --arg cwd "$PWD" '{cwd: $cwd, stop_hook_active: false}' | "$prctl" hook stop)
check "another review of a settled head is looked at again" "$out" "must be fixed"
echo '{"check_runs": []}' >"$state/checks.json"

# At the cap, a change after the whole review goes back; findings only to answer do not.
count_from_here
p1=$(commit cap-p1) && git push -q && set_pr "$p1" && "$prctl" review "$p1" - <<<"$proposal" >/dev/null
p2=$(commit cap-p2) && git push -q && set_pr "$p2" && "$prctl" review "$p2" - <<<"$proposal" >/dev/null
lacks "at the cap, findings only to answer do not go back" "$(cap_stop)" "not answered: 1" "hand it back"
count_from_here
p3=$(commit cap-p3) && git push -q && set_pr "$p3" && "$prctl" review "$p3" - <<<"$proposal" >/dev/null
p4=$(commit cap-p4) && git push -q && set_pr "$p4" && "$prctl" review "$p4" - <<<"$proposal" >/dev/null
p5=$(commit cap-p5) && git push -q && set_pr "$p5" && "$prctl" review "$p5" - <<<"$nothing" >/dev/null
check "at the cap, a change after the whole review goes back" "$(cap_stop)" "hand it back"

# The normal way: reviews of the whole PR only; fixes of the findings of a whole review that found
# nothing that must be fixed are left unreviewed; it goes back once whole reviews in a row found
# something that must be fixed, however many there were before.
way=normal
n1=$(commit normal-a) && git push -q && set_pr "$n1"
check "status tells the way of reviewing" "$("$prctl" status)" "reviewed:  the normal way"
check "the normal way takes no review since the last" "$("$prctl" review "$n1" - <<<"$nothing" 2>&1 || true)" "reviewed the normal way, whole every time"
"$prctl" review "$n1" - <<<"$proposal" >/dev/null
n2=$(commit normal-b) && git push -q && set_pr "$n2"
check "the normal way offers to leave fixes of findings unreviewed" "$("$prctl" unreviewed)" "only fixes findings of the last review of the whole PR"
"$prctl" review --minor "$n2" - <<<"提案の直し" >/dev/null
"$prctl" answer "$n1" - <<<'[{"finding": 1, "action": "直した", "commit": "x"}]' >/dev/null
out=$(stop_hook)
check "fixes left unreviewed after a whole review without a must let the PR end" "[$out]" "[]"
check "the streak is three reviews in a row by default" "$(grep -o 'PRCTL_MUST_STREAK:-[0-9]*' "$prctl")" "PRCTL_MUST_STREAK:-3"
count_from_here
streak_stop() { rm -f "$(git rev-parse --git-path prctl-settled)"; jq -n --arg cwd "$PWD" '{cwd: $cwd, stop_hook_active: false}' | PRCTL_FINAL_CAP=1 PRCTL_MUST_STREAK=2 "$prctl" hook stop; }
m1=$(commit normal-m1) && git push -q && set_pr "$m1" && "$prctl" review "$m1" - <<<"$must" >/dev/null
lacks "the normal way does not count reviews against the cap" "$(streak_stop)" "must be fixed" "hand it back"
m2=$(commit normal-m2) && git push -q && set_pr "$m2" && "$prctl" review "$m2" - <<<"$proposal" >/dev/null
m3=$(commit normal-m3) && git push -q && set_pr "$m3" && "$prctl" review "$m3" - <<<"$must" >/dev/null
lacks "a whole review without a must breaks the streak" "$(streak_stop)" "must be fixed" "hand it back"
m4=$(commit normal-m4) && git push -q && set_pr "$m4" && "$prctl" review "$m4" - <<<"$must" >/dev/null
check "whole reviews with a must in a row send the PR back" "$(streak_stop)" "the last 2 reviews of the whole PR in a row found something that must be fixed"
m5=$(commit normal-m5) && git push -q && set_pr "$m5"
check "the normal way refuses to leave fixes of a review with a must unreviewed" "$("$prctl" review --minor "$m5" - <<<"直し" 2>&1 || true)" "have the whole PR reviewed again"
lacks "the normal way does not offer to leave fixes of a review with a must unreviewed" "$("$prctl" unreviewed)" "after reviewing the whole PR" "prctl review --minor"
check "the normal way shows the range of the whole PR" "$("$prctl" unreviewed)" "the whole PR is $(git merge-base origin/main "$m5")..$m5"
out=$(stop_hook)
lacks "the hook asks the normal way for a review of the whole PR" "$out" "review the whole PR, at every perspective" "review that range"
"$prctl" review "$m5" - <<<"$whole" >/dev/null
check "status tells the way of reviewing" "$("$prctl" status)" "reviewed:  the normal way"
check "prctl mode tells the way" "$("$prctl" mode)" "reviewed the normal way"
"$prctl" mode thorough >/dev/null
check "prctl mode thorough labels the PR" "$(jq -c '[.labels[].name]' "$state/pr.json")" '["念入りのレビュー"]'
check "the label is made first" "$(cat "$state/labels-made")" "name=念入りのレビュー"
check "a labelled PR is reviewed the thorough way" "$("$prctl" mode)" "reviewed the thorough way"
"$prctl" mode normal >/dev/null
check "prctl mode normal takes the label off" "$(jq -c '[.labels[].name]' "$state/pr.json")" '[]'

((failures == 0)) || { echo "$failures failed"; exit 1; }
