# Shell helpers for the hooks, sourced by them.
# shellcheck shell=bash

# shell_words <command> [<mark>]: the command without what is not shell syntax: here-document bodies,
# quoted strings and comments, read left to right once, so that one kind of quote inside another does
# not end it. Quoted parts become the mark, a space unless given.
shell_words() {
  # On standard input, as an environment variable has a limit of its length.
  printf '%s' "$1" | awk -v q="${2:- }" 'BEGIN {
    s = ""; k = 0
    while ((getline l) > 0) s = s (k++ ? "\n" : "") l
    n = length(s); st = "plain"; out = ""; pend = ""; arith = 0
    for (i = 1; i <= n; i++) {
      c = substr(s, i, 1)
      if (st == "single") { if (c == "\047") st = "plain"; continue }
      if (st == "double") { if (c == "\\") i++; else if (c == "\"") st = "plain"; continue }
      if (st == "ansi") { if (c == "\\") i++; else if (c == "\047") st = "plain"; continue }
      if (st == "comment") { if (c == "\n") { st = "plain"; i-- }; continue }
      if (c == "$" && substr(s, i + 1, 1) == "\047") { st = "ansi"; out = out q; i++; continue }
      if (c == "\047") { st = "single"; out = out q; continue }
      if (c == "\"") { st = "double"; out = out q; continue }
      if (c == "\\") { i++; out = out q; continue }
      if (c == "#" && (i == 1 || substr(s, i - 1, 1) ~ /[ \t\n;&|(]/)) { st = "comment"; continue }
      # In arithmetic, (( ... )), << is a shift, not a here-document.
      if (substr(s, i, 2) == "((") { arith++; out = out "(("; i++; continue }
      if (substr(s, i, 2) == "))" && arith > 0) { arith--; out = out "))"; i++; continue }
      if (substr(s, i, 2) == "<<" && substr(s, i, 3) != "<<<" && arith == 0) {
        rest = substr(s, i + 2); strip = 0
        if (substr(rest, 1, 1) == "-") { strip = 1; rest = substr(rest, 2) }
        if (match(rest, /^[ \t]*["\047]?[A-Za-z_][A-Za-z0-9_]*["\047]?/)) {
          w = substr(rest, RSTART, RLENGTH); gsub(/[ \t"\047]/, "", w); pend = w; pstrip = strip
        }
        out = out "<<"; i++; continue
      }
      if (c == "\n" && pend != "") {
        # The body of a here-document, up to its delimiter.
        out = out c; j = i + 1
        while (j <= n) {
          e = index(substr(s, j), "\n"); line = e ? substr(s, j, e - 1) : substr(s, j)
          j += e ? e : length(line) + 1
          if (pstrip) sub(/^\t+/, "", line)
          if (line == pend) break
        }
        i = j - 1; pend = ""; continue
      }
      out = out c
    }
    printf "%s", out
  }'
}
