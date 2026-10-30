# How prctl and check-issues read the Markdown of a PR's or an issue's body, so that both tell its
# headings and code blocks alike.

# body_lines: the lines of a body (the input, as one string), each with whether it is in a code block
# (a fence line is) and the name of the section it heads, if it is a heading of ## outside one. A
# heading may end in spaces or CR.
def body_lines: [foreach (split("\n")[]) as $l ({fence: false};
    if $l | test("^\\s*(```|~~~)") then .fence = (.fence | not) | .line = $l | .head = null | .code = true
    else .line = $l | .code = .fence | .head = (if .fence | not then ($l | capture("^## (?<n>.*?)\\s*$").n // null) else null end) end;
    {line, head, code})];

# scope_items: the items of the scope of an issue's body (the input, as one string; issues.md: scope),
# the task list of its section ## 範囲 outside code blocks, each with whether it is checked and the
# number at its end, the PR that handles it (issues.md: scope-items); null without the section. Only a
# number of this repository counts: #N after the start of the item or a character that cannot be part
# of owner/repo, so an item ending in owner/repo#N names no PR, nor one ending in a word such as x#5.
def scope_items:
  body_lines as $lines
  | ([$lines | to_entries[] | select(.value.head == "範囲") | .key] | first) as $at
  | if $at == null then null else
      ([$lines | to_entries[] | select(.key > $at and .value.head != null) | .key] | first // ($lines | length)) as $end
      | [$lines[$at + 1:$end][] | select(.code | not) | .line
         | capture("^\\s*[-*+] \\[(?<mark>[ xX])\\]\\s+(?<text>.*?)\\s*$")
         | {done: (.mark != " "), text, pr: (.text | capture("(^|[^A-Za-z0-9_/.-])#(?<n>[0-9]+)$").n // null)}]
    end;
