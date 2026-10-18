#!/usr/bin/env bash
# Exercises check-protection against a fake gh that serves the rules, the CODEOWNERS and its
# errors, and the rulesets from files.
set -euo pipefail

cmd=$(cd "$(dirname "$0")/.." && pwd)/bin/check-protection
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export TMPDIR=$tmp

state=$tmp/gh
mkdir -p "$state" "$tmp/bin"
cat >"$tmp/bin/gh" <<'GH'
#!/usr/bin/env bash
set -euo pipefail
dir=$FAKE_GH
args=" $* "
case $args in
  *" repos/o/r/rules/branches/main "*) [[ ! -f $dir/fail-rules ]] || exit 1; cat "$dir/rules.json" ;;
  *" repos/o/r/codeowners/errors?ref="*)
    if [[ -f $dir/errors.json ]]; then cat "$dir/errors.json"
    elif [[ -f $dir/fail-errors ]]; then echo "gh: Server Error (HTTP 500)" >&2; exit 1
    else echo "gh: Not Found (HTTP 404)" >&2; exit 1; fi
    ;;
  *" repos/o/r/contents/.github/CODEOWNERS?ref="*)
    [[ ! -f $dir/fail-contents ]] || { echo "gh: Server Error (HTTP 500)" >&2; exit 1; }
    [[ -f $dir/CODEOWNERS ]] || { echo "gh: Not Found (HTTP 404)" >&2; exit 1; }
    cat "$dir/CODEOWNERS"
    ;;
  *" repos/o/r/contents/"*) echo "gh: Not Found (HTTP 404)" >&2; exit 1 ;;
  *" repos/o/r/rulesets/1 "*) cat "$dir/ruleset.json" ;;
  *" repos/o/r/rulesets/2 "*) cat "$dir/ruleset2.json" ;;
  *" repos/o/r --jq .default_branch "*) echo main ;;
  *) echo "unexpected: $*" >&2; exit 3 ;;
esac
GH
chmod +x "$tmp/bin/gh"
export PATH=$tmp/bin:$PATH FAKE_GH=$state
unset GITHUB_REPOSITORY GITHUB_SHA

failures=0
pass() { echo "ok   $1"; }
fail() { echo "FAIL $1"; failures=$((failures + 1)); }

# A repository protected as the guide sets it up, for a personal repository.
actions_check='[{"context": "check", "integration_id": 15368}]'
rules() {
  jq -n --argjson pr "$1" --argjson checks "${2:-$actions_check}" '[
    {type: "deletion", ruleset_id: 1},
    {type: "non_fast_forward", ruleset_id: 1},
    {type: "pull_request", ruleset_id: 1, parameters: ({required_approving_review_count: 1,
      dismiss_stale_reviews_on_push: true, require_code_owner_review: true, required_reviewers: []} + $pr)},
    {type: "required_status_checks", ruleset_id: 1, parameters: {required_status_checks: $checks}}]' >"$state/rules.json"
}
reset() {
  rm -f "$state"/*
  rules '{}'
  echo '{"errors": []}' >"$state/errors.json"
  printf '# Humans only.\n* @someone\n' >"$state/CODEOWNERS"
  echo '{"id": 1}' >"$state/ruleset.json"
}
run() { out=$("$cmd" o/r 2>&1) && rc=0 || rc=$?; }
expect() { # <name> <exit code> [<text in the output>]
  if [[ $rc == "$2" && ( -z ${3:-} || $out == *"$3"* ) ]]; then pass "$1"; else fail "$1 (exit $rc)"; printf '     %s\n' "${out//$'\n'/$'\n'     }"; fi
}

reset; run; expect "a personal repository set up as the guide passes" 0 "approvals count only from the code owners"
reset; run; expect "bypass that cannot be read is told, not failed" 0 "cannot be read with this token"

reset; jq 'map(select(.type != "deletion"))' "$state/rules.json" >"$state/x" && mv "$state/x" "$state/rules.json"; run
expect "a branch that can be deleted fails" 1 "can be deleted"
reset; jq 'map(select(.type != "non_fast_forward"))' "$state/rules.json" >"$state/x" && mv "$state/x" "$state/rules.json"; run
expect "a branch that can be force-pushed fails" 1 "can be force-pushed"
reset; jq 'map(select(.type != "pull_request"))' "$state/rules.json" >"$state/x" && mv "$state/x" "$state/rules.json"; run
expect "no pull request rule fails" 1 "not limited to humans"
reset; rules '{"required_approving_review_count": 0}'; run; expect "no approval required fails" 1 "needs no approval"
reset; rules '{"dismiss_stale_reviews_on_push": false}'; run; expect "approvals kept after a push fail" 1 "stay after a push"
reset; rules '{"require_code_owner_review": false}'; run; expect "CODEOWNERS without code owner review fails" 1 "not limited to humans"

reset; echo '{"errors": [{"line": 2, "kind": "Unknown owner", "path": ".github/CODEOWNERS", "message": "Unknown owner on line 2"}]}' >"$state/errors.json"; run
expect "CODEOWNERS with errors fails" 1 "Unknown owner on line 2"
reset; printf '* @someone\ndocs/\n' >"$state/CODEOWNERS"; run
expect "a rule without owners fails" 1 "rules without owners"
reset; printf 'docs/ @someone\n' >"$state/CODEOWNERS"; run
expect "CODEOWNERS not starting with '*' fails" 1 "first rule of CODEOWNERS is not '*'"
reset; printf '* @someone # the owner\n' >"$state/CODEOWNERS"; run
expect "a comment after the owners is not an owner nor a missing one" 0
reset; rm "$state/CODEOWNERS"; run; expect "CODEOWNERS reported but in none of its places stops with 2" 2 "in none of its places"
reset; touch "$state/fail-contents"; run; expect "CODEOWNERS that cannot be read stops with 2" 2 ".github/CODEOWNERS at"
reset; printf '# only a comment\n' >"$state/CODEOWNERS"; run; expect "CODEOWNERS of comments only fails with an NG line" 1 "NG   CODEOWNERS at main has no rules"
reset; : >"$state/CODEOWNERS"; run; expect "an empty CODEOWNERS fails as having no rules" 1 "has no rules"

team='{"require_code_owner_review": false, "required_reviewers": [{"minimum_approvals": 1, "file_patterns": ["*"], "reviewer": {"id": 7, "type": "Team"}}]}'
reset; rm "$state/errors.json" "$state/CODEOWNERS"; rules "$team"; run
expect "an organization repository with a required team passes" 0 "required team on every file"
reset; rm "$state/errors.json" "$state/CODEOWNERS"; rules "${team/\"minimum_approvals\": 1/\"minimum_approvals\": 0}"; run
expect "a required team with no approvals fails" 1 "not limited to humans"
reset; rm "$state/errors.json" "$state/CODEOWNERS"; rules "${team/\[\"\*\"\]/[\"docs\/**\"]}"; run
expect "a required team on some files only fails" 1 "not limited to humans"
reset; rm "$state/errors.json" "$state/CODEOWNERS"; rules '{"require_code_owner_review": false}'; run
expect "neither CODEOWNERS nor a team fails" 1 "not limited to humans"
reset; printf 'docs/ @someone\n' >"$state/CODEOWNERS"; rules "$team"; run
expect "a CODEOWNERS left beside a required team is only told about" 0 "--   the first rule of CODEOWNERS"
reset; jq '. + [{type: "pull_request", ruleset_id: 2, parameters: {required_approving_review_count: 0, dismiss_stale_reviews_on_push: true}}]
  | map(if .type == "pull_request" and .ruleset_id == 1 then .parameters.dismiss_stale_reviews_on_push = false else . end)' \
  "$state/rules.json" >"$state/x" && mv "$state/x" "$state/rules.json"
echo '{"id": 2}' >"$state/ruleset2.json"; run
expect "requirements split over two rulesets pass" 0 "approvals are dismissed on push"
reset; rm "$state/errors.json"; touch "$state/fail-errors"; run
expect "CODEOWNERS errors that cannot be read stop with 2" 2

reset; rules '{}' '[]'; run; expect "no required check fails" 1 "'check' is not required"
reset; rules '{}' '[{"context": "check", "integration_id": 15368}, {"context": "check", "integration_id": 99}]'; run
expect "a required check also from another source fails" 1 "not only from GitHub Actions"
reset; rules '{}' '[{"context": "check"}]'; run; expect "a required check from any source fails" 1 "not only from GitHub Actions"

admin='{"id": 1, "bypass_actors": [{"actor_id": 5, "actor_type": "RepositoryRole", "bypass_mode": "pull_request"}]}'
reset; echo "$admin" >"$state/ruleset.json"; run; expect "bypass by admins for pull requests passes" 0 "only by admins"
reset; echo "${admin/pull_request/always}" >"$state/ruleset.json"; run; expect "bypass always fails" 1 "RepositoryRole 5 always"
reset; echo "${admin/\"actor_id\": 5/\"actor_id\": 4}" >"$state/ruleset.json"; run; expect "bypass by a role that writes fails" 1 "RepositoryRole 4"
reset; echo "${admin/RepositoryRole/Integration}" >"$state/ruleset.json"; run; expect "bypass by an app fails" 1 "Integration 5"
orgadmin='{"id": 1, "bypass_actors": [{"actor_type": "OrganizationAdmin", "bypass_mode": "pull_request"}]}'
reset; echo "$orgadmin" >"$state/ruleset.json"; run; expect "bypass by organization admins for pull requests passes" 0 "only by admins"
reset; echo "${orgadmin/pull_request/always}" >"$state/ruleset.json"; run; expect "bypass always by organization admins fails" 1 "OrganizationAdmin - always"

reset; touch "$state/fail-rules"; run; expect "rules that cannot be read stop with 2" 2

[[ $failures == 0 ]] || { echo "$failures failed"; exit 1; }
