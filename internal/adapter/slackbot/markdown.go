package slackbot

import (
	"regexp"
	"strings"
)

// slackMarkdown rewrites standard Markdown where Slack's markdown block reads it differently, in
// the ways that keep its meaning: bare URLs are put in angle brackets, and a table right under a
// line of text gets a blank line before it. What Slack does with each form is in the slack
// plugin's reference (platform/plugins/slack/docs/reference/markdown.md).
func slackMarkdown(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	var fence string
	for i, line := range lines {
		if fence != "" {
			if closesFence(line, fence) {
				fence = ""
			}
			out = append(out, line)
			continue
		}
		if f := opensFence(line); f != "" {
			fence = f
			out = append(out, line)
			continue
		}
		if i > 0 && strings.TrimSpace(lines[i-1]) != "" && !isTableRow(lines[i-1]) && startsTable(lines, i) {
			out = append(out, "")
		}
		out = append(out, bracketURLs(line))
	}
	return strings.Join(out, "\n")
}

// opensFence gives the fence a line opens (its run of backticks or tildes), or "" if it opens none.
// Fences are taken at any indent, since a fence in a list item is indented.
func opensFence(line string) string {
	s := strings.TrimLeft(line, " \t")
	for _, c := range []string{"`", "~"} {
		n := len(s) - len(strings.TrimLeft(s, c))
		if n >= 3 && (c == "~" || !strings.Contains(s[n:], "`")) {
			return s[:n]
		}
	}
	return ""
}

func closesFence(line, fence string) bool {
	s := strings.TrimSpace(line)
	return strings.HasPrefix(s, fence) && strings.Trim(s, fence[:1]) == ""
}

var delimiterRow = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)

// startsTable tells whether lines[i] is the header row of a table, which the delimiter row
// (| --- | --- |) follows.
func startsTable(lines []string, i int) bool {
	return i+1 < len(lines) && isTableRow(lines[i]) && strings.Contains(lines[i+1], "|") && delimiterRow.MatchString(lines[i+1])
}

func isTableRow(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "|")
}

// bracketURLs puts each bare URL of a line in angle brackets. Slack links a bare URL only after
// a space or ASCII punctuation, and takes the characters up to the next space into it, so a URL
// next to Japanese text is either not linked or linked with the text. A URL ends at a space, a
// non-ASCII character, or a character a URL does not hold here (< > ` " and the like), and loses
// the punctuation that ends a sentence. Code spans, autolinks, link texts and link destinations
// stay as they are: a link cannot hold another link.
func bracketURLs(line string) string {
	var b strings.Builder
	linkText := 0
	for i := 0; i < len(line); {
		switch {
		case line[i] == '[':
			linkText++
		case line[i] == ']' && linkText > 0:
			linkText--
		case line[i] == '`':
			n := runLength(line, i, '`')
			if end := strings.Index(line[i+n:], line[i:i+n]); end >= 0 {
				end += i + 2*n
				b.WriteString(line[i:end])
				i = end
				continue
			}
			b.WriteString(line[i : i+n])
			i += n
			continue
		case linkText == 0 && !linkedAlready(line, i):
			if n := schemeLength(line[i:]); n > 0 {
				if end := urlEnd(line, i); end > i+n {
					b.WriteString("<" + line[i:end] + ">")
					i = end
					continue
				}
			}
		}
		b.WriteByte(line[i])
		i++
	}
	return b.String()
}

func runLength(s string, i int, c byte) int {
	n := 0
	for i+n < len(s) && s[i+n] == c {
		n++
	}
	return n
}

func schemeLength(s string) int {
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(s, scheme) {
			return len(scheme)
		}
	}
	return 0
}

// linkedAlready tells whether the URL at i is already an autolink (<URL>) or a link destination
// ([text](URL)).
func linkedAlready(line string, i int) bool {
	if i == 0 {
		return false
	}
	switch line[i-1] {
	case '<':
		return true
	case '(':
		return i >= 2 && line[i-2] == ']'
	}
	return false
}

func urlEnd(line string, i int) int {
	end := i
	for end < len(line) && isURLByte(line[end]) {
		end++
	}
	for end > i {
		c := line[end-1]
		if strings.IndexByte(".,:;!?'*_~", c) >= 0 || (c == ')' && strings.Count(line[i:end], "(") < strings.Count(line[i:end], ")")) {
			end--
			continue
		}
		break
	}
	return end
}

func isURLByte(c byte) bool {
	return c > ' ' && c < 0x7f && strings.IndexByte("<>`\"{}|\\^", c) < 0
}
