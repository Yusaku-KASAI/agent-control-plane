# How prctl and check-issues read the Markdown of a PR's or an issue's body, so that both tell its
# headings and code blocks alike.

# body_lines: the lines of a body (the input, as one string), each with whether it is in a code block
# (a fence line is) and the name of the section it heads, if it is a heading of ## outside one. A
# heading may end in spaces or CR.
def body_lines: [foreach (split("\n")[]) as $l ({fence: false};
    if $l | test("^\\s*(```|~~~)") then .fence = (.fence | not) | .line = $l | .head = null | .code = true
    else .line = $l | .code = .fence | .head = (if .fence | not then ($l | capture("^## (?<n>.*?)\\s*$").n // null) else null end) end;
    {line, head, code})];
