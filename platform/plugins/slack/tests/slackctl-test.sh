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
check "a bare URL after Japanese is found" 1 "$(found 'PR：https://x.example/pull/1')"
check "a URL in angle brackets passes" 0 "$(found 'PR：<https://x.example/pull/1> と [PR](https://x.example)')"
check "a URL in code passes" 0 "$(found '`https://x.example`、後')"
check "bold opening on ASCII punctuation after Japanese is found" 1 "$(found '、**#21 見出し**：後')"
check "bold closing on code before Japanese is found" 1 "$(found '**`code`**：後')"
check "bold with spaces outside, or without punctuation inside, passes" 0 "$(found '前 **`code`** 後、前**太字**後、**太字**(注)')"
check "a table right under text is found" 1 "$(found $'表です\n| a | b |\n| --- | --- |')"
check "a table after a blank line passes" 0 "$(found $'表です\n\n| a | b |\n| --- | --- |')"
check "an ordered item after a nested list is found" 1 "$(found $'1. 一\n   - 子\n2. 二')"
check "an ordered list split by a paragraph passes" 0 "$(found $'1. 一\n\n段落\n\n2. 二')"
check "formatting in a heading is found" 1 "$(found '## **見出し**')"
check "an entity reference, which may be meant, passes" 0 "$(found '`&amp;` の件')"
check "a code block in a list item is found" 1 "$(found $'- 項目\n  ```\n  code\n  ```')"
check "the text after a code block is read" 1 "$(found $'```\ncode\n```\nPR：https://x.example')"
check "the inside of a code block is not read" 0 "$(found $'```\nPR：https://x.example\n## **x**\n```')"

# post: the forms Slack reads differently stop the post unless --force.
echo '{"ok": true, "channel": "C1", "ts": "9.0"}' >"$tmp/api/chat.postMessage.json"
got=$("$cmd" post C1 <<<'PR：https://x.example' 2>&1 || true)
check "post refuses the forms Slack reads differently" "yes" "$([[ $got == *"--force"* && $got != *posted* ]] && echo yes || echo no)"
got=$("$cmd" post --force C1 <<<'PR：https://x.example')
check "post --force posts them" "posted C1/9.0" "$got"
got=$("$cmd" post C1 1.0 <<<'PR：<https://x.example>')
check "post posts text that reads well" "posted C1/9.0" "$got"

((failures == 0))
