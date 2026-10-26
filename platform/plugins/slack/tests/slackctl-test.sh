#!/usr/bin/env bash
# shellcheck disable=SC2016 # the backticks in single quotes are Markdown's inline code
# Exercises slackctl against a fake curl that serves Web API responses from files.
set -euo pipefail

cmd=$(cd "$(dirname "$0")/.." && pwd)/bin/slackctl
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/bin" "$tmp/api"
cat >"$tmp/bin/curl" <<'CURL'
#!/usr/bin/env bash
set -euo pipefail
url=${*: -1}
cat "$FAKE_API/${url##*/}.json"
CURL
chmod +x "$tmp/bin/curl"
export PATH=$tmp/bin:$PATH FAKE_API=$tmp/api SLACK_BOT_TOKEN=xoxb-test
unset SLACK_BOT_TOKEN_FILE SLACK_WEBHOOK_URL

failures=0
check() {
  local name=$1 want=$2 got=$3
  if [[ $got == "$want" ]]; then echo "ok   $name"; else echo "FAIL $name: got '$got', want '$want'"; failures=$((failures + 1)); fi
}

cat >"$tmp/api/conversations.replies.json" <<'JSON'
{"ok": true, "messages": [
  {"ts": "1.0", "user": "U1", "text": "<@U2> a &amp; b, `theme:&lt;名前&gt;`, &amp;lt; and <https://example.com|link>"}
]}
JSON
got=$("$cmd" thread C1 1.0 | sed -n 2p)
check "the text is printed as Slack shows it, keeping Slack's markup" \
  '<@U2> a & b, `theme:<名前>`, &lt; and <https://example.com|link>' "$got"

cat >"$tmp/api/conversations.replies.json" <<'JSON'
{"ok": true, "messages": [
  {"ts": "1.0", "user": "U1", "files": []},
  {"ts": "2.0", "user": "U1", "thread_ts": "1.0", "text": "next"}
]}
JSON
got=$("$cmd" thread C1 1.0 | sed -n 5p)
check "a message without text does not stop the thread" "next" "$got"

# check: the forms Slack reads differently are found, line by line; the forms that read well pass.
found() { "$cmd" check 2>&1 >/dev/null <<<"$1" | grep -c '^line' || true; }
lines() { "$cmd" check 2>&1 >/dev/null <<<"$1" | sed -n 's/^line \([0-9]*\):.*/\1/p' | paste -sd, -; }
cases=$(jq -c '.[]' "$(dirname "$cmd")/../tests/markdown-cases.json")
test -n "$cases"
while IFS= read -r c; do
  check "$(jq -r .name <<<"$c")" "$(jq -r '.found | join(",")' <<<"$c")" "$(lines "$(jq -r .text <<<"$c")")"
done <<<"$cases"

# Each finding starts as the reference says, for others to tell the forms by; each start is found.
reference=$(dirname "$cmd")/../docs/reference/check-markdown.md
# shellcheck disable=SC2016 # backquotes of Markdown
mapfile -t starts < <(sed -n '/^## 文の書き出し/,/^## /p' "$reference" | sed -n 's/^| `\([^`]*\)` |.*/\1/p')
check "the reference tells the starts" 6 "${#starts[@]}"
messages=$(while IFS= read -r c; do "$cmd" check 2>&1 >/dev/null <<<"$(jq -r .text <<<"$c")" | sed -n 's/^line [0-9]*: //p' || true; done <<<"$cases")
for s in "${starts[@]}"; do
  check "a case finds what starts with \"$s\"" yes "$(grep -q "^$s" <<<"$messages" && echo yes || echo no)"
done
unknown=$messages
for s in "${starts[@]}"; do unknown=$(grep -v "^$s" <<<"$unknown" || true); done
check "every finding starts as the reference says" "" "$unknown"

got=$("$cmd" check <<<'PR：https://x.example' 2>/dev/null && echo passed || echo failed)
check "check fails on the forms Slack reads differently" failed "$got"
check "check passes text that reads well" ok "$("$cmd" check <<<'PR：<https://x.example>')"

# A failure of the check is not taken for the forms Slack reads differently.
mkdir -p "$tmp/broken"
printf '#!/usr/bin/env bash\nexit 1\n' >"$tmp/broken/python3"
chmod +x "$tmp/broken/python3"
got=$(PATH=$tmp/broken:$PATH "$cmd" check <<<'text' 2>&1 || true)
check "a failing check-markdown is told as a failure" "slackctl: check-markdown failed (exit 1)" "$got"
check "broken UTF-8 is read" 1 "$(found "$(printf 'PR：https://x.example\xff')")"

# post: the forms Slack reads differently stop the post unless --force.
echo '{"ok": true, "channel": "C1", "ts": "9.0"}' >"$tmp/api/chat.postMessage.json"
got=$("$cmd" post C1 <<<'PR：https://x.example' 2>&1 || true)
check "post refuses the forms Slack reads differently" "yes" "$([[ $got == *"--force"* && $got != *posted* ]] && echo yes || echo no)"
got=$("$cmd" post --force C1 <<<'PR：https://x.example')
check "post --force posts them" "posted C1/9.0" "$got"
got=$("$cmd" post C1 1.0 --force <<<'PR：https://x.example')
check "post takes --force after the channel and thread" "posted C1/9.0" "$got"
got=$("$cmd" post C1 1.0 <<<'PR：<https://x.example>')
check "post posts text that reads well" "posted C1/9.0" "$got"
got=$(PATH=$tmp/broken:$PATH "$cmd" post C1 <<<'text' 2>&1 || true)
check "post stops on a failing check-markdown" "slackctl: check-markdown failed (exit 1)" "$got"
got=$(PATH=$tmp/broken:$PATH "$cmd" post --force C1 <<<'text')
check "post --force posts without a working check-markdown" "posted C1/9.0" "$got"

# What can be posted at all: something, within Slack's limit for one message.
check "nothing is not posted" "slackctl: nothing to post" "$("$cmd" post C1 <<<$' \n ' 2>&1 || true)"
long=$(printf 'a%.0s' $(seq 12001))
check "a text over the limit is not posted" "yes" "$(got=$("$cmd" post C1 <<<"$long" 2>&1 || true); [[ $got == *"12001 characters"* ]] && echo yes || echo no)"
check "a text at the limit is posted" "posted C1/9.0" "$("$cmd" post C1 <<<"${long:1}")"

# The token comes from its file, read each time, before the variable.
printf 'xoxb-file\n' >"$tmp/token"
check "the token file is read" "read: api (bot token)" "$(env -u SLACK_BOT_TOKEN SLACK_BOT_TOKEN_FILE="$tmp/token" "$cmd" status | head -1)"
cat >"$tmp/bin/curl" <<'CURL'
#!/usr/bin/env bash
set -euo pipefail
url=${*: -1}
printf '%s\n' "$*" >"$FAKE_API/last"
cat "$FAKE_API/${url##*/}.json"
CURL
"$cmd" post C1 <<<'text' >/dev/null
check "the token of the variable is used without a file" "yes" "$(grep -q 'Bearer xoxb-test' "$tmp/api/last" && echo yes || echo no)"
SLACK_BOT_TOKEN_FILE="$tmp/token" "$cmd" post C1 <<<'text' >/dev/null
check "the token of the file comes before the variable" "yes" "$(grep -q 'Bearer xoxb-file' "$tmp/api/last" && echo yes || echo no)"

# Without a bot token, an incoming webhook posts to its own channel only.
echo ok >"$tmp/api/hook.json"
hook() { env -u SLACK_BOT_TOKEN SLACK_WEBHOOK_URL=https://hooks.invalid/hook "$cmd" "$@" 2>&1 || true; }
check "a webhook tells what it can post" "post: incoming webhook: its fixed channel only, no threads" "$(hook status | tail -1)"
check "a webhook posts" "posted to the webhook" "$(hook post <<<'text')"
check "a webhook refuses a channel" "slackctl: an incoming webhook posts to its own channel; give no channel or thread" "$(hook post C1 <<<'text')"
check "a webhook refuses a thread" "slackctl: an incoming webhook posts to its own channel; give no channel or thread" "$(hook post C1 1.0 <<<'text')"
check "a webhook refuses a thread without a channel" "slackctl: an incoming webhook posts to its own channel; give no channel or thread" "$(hook post '' 1.0 <<<'text')"
check "nothing to post with is told" "slackctl: no bot token or webhook to post with" "$(env -u SLACK_BOT_TOKEN "$cmd" post C1 <<<'text' 2>&1 || true)"

((failures == 0))
