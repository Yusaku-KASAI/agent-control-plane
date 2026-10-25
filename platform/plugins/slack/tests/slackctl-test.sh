#!/usr/bin/env bash
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
# shellcheck disable=SC2016 # the backticks are the message's inline code
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

((failures == 0))
