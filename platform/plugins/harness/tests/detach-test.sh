#!/usr/bin/env bash
# Checks that the detach hook refuses, only under the control plane, Bash commands that let a
# process go and the tools that watch for an end, and leaves everything else alone.
set -euo pipefail

hook=$(cd "$(dirname "$0")/.." && pwd)/scripts/detach
failed=0
# decision <command>: what the hook decides for a Bash command under the control plane.
decision() {
  local out
  # The command on standard input, as an argument has a limit of its length.
  out=$(printf '%s' "$1" | jq -Rs '{tool_name: "Bash", tool_input: {command: .}}' | ACP_SESSION_BRANCH=acp/x "$hook")
  if [[ -z $out ]]; then echo allow; else jq -r .hookSpecificOutput.permissionDecision <<<"$out"; fi
}
check() {
  local name=$1 want=$2 got=$3
  if [[ $got == "$want" ]]; then echo "ok   $name"; else echo "FAIL $name: got '$got', want '$want'"; failed=1; fi
}

while IFS='|' read -r want command; do
  check "$want: $command" "$want" "$(decision "$command")"
done <<'CASES'
deny|prctl ci --wait &
deny|sleep 10 & echo started
deny|nohup make serve
deny|make serve & disown
deny|setsid ./run
deny|(cd x && make) &
allow|make check
allow|make check && git push
allow|make 2>&1 | tail
allow|make &>/tmp/log
allow|cmd >&2
allow|cmd 2>&-
allow|a & b & wait
allow|echo "a & b"
allow|echo 'nohup here'
allow|git commit -m "fix &"
allow|ls # sleep & comment
allow|echo disowned-thing
deny|echo "it's"; prctl ci --wait & echo 'done'
deny|echo "don't" & echo 'ok'
deny|wait; prctl ci --wait &
deny|prctl ci --wait & echo wait
allow|a & b; wait
allow|for f in x; do a & done; wait
allow|make & make test && wait
deny|a & echo undo wait
deny|a & b |& wait
deny|a & b | wait
CASES
arith=$'echo $((1<<x))\nsleep 9 &\nx'
check "a shift in arithmetic is not a here-document" deny "$(decision "$arith")"
big="cat >f <<'EOF'"$'\n'"$(head -c 200000 /dev/zero | tr '\0' 'x')"$'\nEOF\nsleep 9 &'
check "a command over the limit of an environment variable is read" deny "$(decision "$big")"
multi=$'git commit -m "title

body: a & b"'
check "a quote over lines is not read" allow "$(decision "$multi")"
multi=$(printf 'gh pr create --body "it%ss fine & good\nmore"' "'")
check "a quote with an apostrophe over lines is not read" allow "$(decision "$multi")"
heredoc=$'cat >f <<\'EOF\'\nsleep 1 &\nnohup x\nEOF\nmake'
check "a here-document is not read as commands" allow "$(decision "$heredoc")"
heredoc=$'cat >f <<-EOF\n\tsleep 1 &\n\tEOF\nsleep 2 &'
check "a command after a here-document is read" deny "$(decision "$heredoc")"
for tool in Monitor ScheduleWakeup CronCreate; do
  check "$tool is denied" deny \
    "$(jq -n --arg t "$tool" '{tool_name: $t, tool_input: {}}' | ACP_SESSION_BRANCH=acp/x "$hook" | jq -r .hookSpecificOutput.permissionDecision)"
  check "$tool is left alone outside the control plane" "" \
    "$(jq -n --arg t "$tool" '{tool_name: $t, tool_input: {}}' | env -u ACP_SESSION_BRANCH "$hook")"
done
check "a let-go command is left alone outside the control plane" "" \
  "$(jq -n '{tool_name: "Bash", tool_input: {command: "sleep 1 &"}}' | env -u ACP_SESSION_BRANCH "$hook")"
exit $failed
