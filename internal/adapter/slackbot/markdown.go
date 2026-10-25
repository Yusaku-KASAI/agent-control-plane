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
	var code *fence
	for i, line := range lines {
		if code != nil {
			if code.holds(line) {
				if code.closedBy(line) {
					code = nil
				}
				out = append(out, line)
				continue
			}
			// The quote or list item of the code block has ended, and the block with it.
			code = nil
		}
		if f, ok := opensFence(line); ok {
			code = &f
			out = append(out, line)
			continue
		}
		if i > 0 && strings.TrimSpace(lines[i-1]) != "" && startsTable(lines, i) {
			out = append(out, "")
		}
		out = append(out, bracketURLs(line))
	}
	return strings.Join(out, "\n")
}

// fence is an open code block: its run of backticks or tildes, how many quote marks hold it, and,
// in a list item or indented, the column of the run after the quote marks. As in CommonMark, the
// block ends with its quote, and in a list item with the item: at a line with fewer quote marks,
// or at a line of text indented less than the run. The same rules are in the slack plugin's
// check-markdown, and both are tried on its cases (platform/plugins/slack/tests/markdown-cases.json).
type fence struct {
	run            string
	quotes, indent int
}

var (
	quoteMark   = regexp.MustCompile(`^ {0,3}> ?`)
	listMarkers = regexp.MustCompile(`^(?:(?:[-*+]|\d+[.)])[ \t]+)*`)
)

// unquote drops up to max quote marks from the start of a line, and tells how many it dropped.
func unquote(line string, max int) (int, string) {
	n := 0
	for n < max {
		m := quoteMark.FindString(line)
		if m == "" {
			break
		}
		line = line[len(m):]
		n++
	}
	return n, line
}

// opensFence gives the code block a line opens, if it opens one. The run may follow quote marks,
// an indent and list markers, since a code block in a list item is indented or starts the item.
func opensFence(line string) (fence, bool) {
	quotes, rest := unquote(line, len(line))
	col := len(rest) - len(strings.TrimLeft(rest, " \t"))
	col += len(listMarkers.FindString(rest[col:]))
	s := rest[col:]
	for _, c := range []string{"`", "~"} {
		n := len(s) - len(strings.TrimLeft(s, c))
		// A backtick run with a backtick after it is inline code (```x```), not a fence.
		if n >= 3 && (c == "~" || !strings.Contains(s[n:], "`")) {
			return fence{run: s[:n], quotes: quotes, indent: col}, true
		}
	}
	return fence{}, false
}

// holds tells whether a line is still within the quote and the list item of the code block.
func (f fence) holds(line string) bool {
	n, rest := unquote(line, f.quotes)
	if n < f.quotes {
		return false
	}
	text := strings.TrimLeft(rest, " \t")
	return text == "" || len(rest)-len(text) >= f.indent
}

// closedBy tells whether a line closes the code block. A closing run carries no list marker, so a
// line like "- ```" in a code block is code.
func (f fence) closedBy(line string) bool {
	_, rest := unquote(line, f.quotes)
	s := strings.TrimSpace(rest)
	return strings.HasPrefix(s, f.run) && strings.Trim(s, f.run[:1]) == ""
}

var delimiterRow = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)

// startsTable tells whether lines[i] is the header row of a table, which the delimiter row
// (| --- | --- |, or --- | --- without the outer bars) follows.
func startsTable(lines []string, i int) bool {
	return i+1 < len(lines) && strings.Contains(lines[i], "|") && strings.Contains(lines[i+1], "|") && delimiterRow.MatchString(lines[i+1])
}

// bracketURLs puts each bare URL of a line in angle brackets. Slack links a bare URL only after
// a space or ASCII punctuation, and takes the characters up to the next space into it, so a URL
// next to Japanese text is either not linked or linked with the text. A URL ends at a space, a
// non-ASCII character, or a character a URL does not hold here (< > ` " and the like), and loses
// the punctuation that ends a sentence. Code spans, autolinks, link texts and link destinations
// stay as they are: a link cannot hold another link.
func bracketURLs(line string) string {
	var b strings.Builder
	// brackets holds, for each [ still open, whether it opens a link text.
	var brackets []bool
	linkText := 0
	for i := 0; i < len(line); {
		switch {
		case line[i] == '[':
			opens := opensLinkText(line, i)
			brackets = append(brackets, opens)
			if opens {
				linkText++
			}
		case line[i] == ']' && len(brackets) > 0:
			if brackets[len(brackets)-1] {
				linkText--
			}
			brackets = brackets[:len(brackets)-1]
		case line[i] == '`':
			end := codeSpanEnd(line, i)
			b.WriteString(line[i:end])
			i = end
			continue
		case linkText == 0 && schemeLength(line[i:]) > 0:
			// The whole URL is passed over, so that a URL inside it (?next=https://…) is not taken
			// for another one.
			end := urlEnd(line, i, len(brackets) > 0)
			if !linkedAlready(line, i) && end > i+schemeLength(line[i:]) {
				b.WriteString("<" + line[i:end] + ">")
			} else {
				b.WriteString(line[i:end])
			}
			i = end
			continue
		}
		b.WriteByte(line[i])
		i++
	}
	return b.String()
}

// codeSpanEnd gives where the code span opening at the backticks at i ends. Backticks that close
// nowhere are text, and only they are passed over.
func codeSpanEnd(line string, i int) int {
	n := runLength(line, i, '`')
	if end := strings.Index(line[i+n:], line[i:i+n]); end >= 0 {
		return i + end + 2*n
	}
	return i + n
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

// opensLinkText tells whether the [ at i opens a link text: its ] is followed by ( or [, as in
// [text](URL) and [text][ref]. Brackets that are not links, such as [注：URL], hold bare URLs too.
func opensLinkText(line string, i int) bool {
	depth := 0
	for j := i; j < len(line); j++ {
		switch line[j] {
		case '`':
			// A code span binds tighter than the brackets of a link, so its ] are not counted.
			j = codeSpanEnd(line, j) - 1
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return j+1 < len(line) && (line[j+1] == '(' || line[j+1] == '[')
			}
		}
	}
	return false
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

// urlEnd gives where the URL at i ends. Within brackets ([注：URL]) the URL ends at a ], which
// otherwise a URL may hold, as in a query (?a[]=1).
func urlEnd(line string, i int, inBrackets bool) int {
	end := i
	open := 0 // the [ opened within the URL
	for ; end < len(line) && isURLByte(line[end]); end++ {
		switch line[end] {
		case '[':
			open++
		case ']':
			if open == 0 && inBrackets {
				return trimURLEnd(line, i, end)
			}
			open = max(open-1, 0)
		}
	}
	return trimURLEnd(line, i, end)
}

// trimURLEnd drops the punctuation that ends a sentence, and a ) the URL does not open, from the
// end of the URL at line[i:end].
func trimURLEnd(line string, i, end int) int {
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
