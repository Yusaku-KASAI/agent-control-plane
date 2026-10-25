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
	var list listItems
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
		f, opens := opensFence(line, &list)
		list.see(line)
		if opens {
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
// in a list item, the column where the item's content starts after the quote marks. As in
// CommonMark, the block ends with its quote, and in a list item with the item: at a line with fewer
// quote marks, or at a line of text indented less than the item's content. The same rules are in
// the slack plugin's check-markdown, and both read the plugin's cases
// (platform/plugins/slack/tests/markdown-cases.json).
type fence struct {
	run            string
	quotes, indent int
}

var (
	quoteMark   = regexp.MustCompile(`^[ \t]*> ?`)
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

// width gives the columns s takes, a tab going on to the next multiple of 4 as in CommonMark.
func width(s string) int {
	w := 0
	for _, c := range s {
		if c == '\t' {
			w += 4 - w%4
		} else {
			w++
		}
	}
	return w
}

func leadLength(s string) int {
	return len(s) - len(strings.TrimLeft(s, " \t"))
}

// indentOf gives the columns of a line's indent.
func indentOf(s string) int {
	return width(s[:leadLength(s)])
}

// listItems follows the list items open at a line, for each quote it is in, outermost first: the
// column where each item's content starts, innermost last. A line of text, or a quote mark,
// indented less than an item's content ends the item; a line of text that carries on an item
// without the indent (a lazy continuation) is taken to end it too. A quote within an item has
// lists of its own, and once it ends, the item's go on.
type listItems struct {
	levels [][]int
}

var listMarker = regexp.MustCompile(`^(?:[-*+]|\d+[.)])[ \t]+`)

// see takes in a line outside code blocks.
func (l *listItems) see(line string) {
	rest, depth := line, 0
	for {
		if len(l.levels) <= depth {
			l.levels = append(l.levels, nil)
		}
		if strings.TrimSpace(rest) == "" {
			// A blank line ends the quotes deeper than it.
			l.levels = l.levels[:depth+1]
			return
		}
		lead := leadLength(rest)
		l.end(depth, width(rest[:lead]))
		m := quoteMark.FindString(rest)
		if m == "" {
			break
		}
		rest, depth = rest[len(m):], depth+1
	}
	l.levels = l.levels[:depth+1]
	at := leadLength(rest)
	for m := listMarker.FindString(rest[at:]); m != ""; m = listMarker.FindString(rest[at:]) {
		at += len(m)
		l.levels[depth] = append(l.levels[depth], width(rest[:at]))
	}
}

// end ends the items of a quote that a line of text at indent leaves.
func (l *listItems) end(depth, indent int) {
	cols := l.levels[depth]
	for len(cols) > 0 && cols[len(cols)-1] > indent {
		cols = cols[:len(cols)-1]
	}
	l.levels[depth] = cols
}

// content gives where the content of the innermost item holding a block at col starts, or 0
// outside any item.
func (l *listItems) content(quotes, col int) int {
	if quotes >= len(l.levels) {
		return 0
	}
	cols := l.levels[quotes]
	for i := len(cols) - 1; i >= 0; i-- {
		if cols[i] <= col {
			return cols[i]
		}
	}
	return 0
}

// opensFence gives the code block a line opens, if it opens one. The run may follow quote marks,
// an indent and list markers, since a code block in a list item is indented or starts the item.
func opensFence(line string, list *listItems) (fence, bool) {
	quotes, rest := unquote(line, len(line))
	lead := leadLength(rest)
	markers := len(listMarkers.FindString(rest[lead:]))
	s := rest[lead+markers:]
	for _, c := range []string{"`", "~"} {
		n := len(s) - len(strings.TrimLeft(s, c))
		// A backtick run with a backtick after it is inline code (```x```), not a fence.
		if n >= 3 && (c == "~" || !strings.Contains(s[n:], "`")) {
			indent := width(rest[:lead+markers])
			if markers == 0 {
				indent = list.content(quotes, width(rest[:lead]))
			}
			return fence{run: s[:n], quotes: quotes, indent: indent}, true
		}
	}
	return fence{}, false
}

// holds tells whether a line is still within the quote and the list item of the code block. A
// closing run indented less than the item still closes the block, unlike in CommonMark, so that
// it does not open another block over the rest of the text.
func (f fence) holds(line string) bool {
	n, rest := unquote(line, f.quotes)
	if n < f.quotes {
		return false
	}
	return strings.TrimSpace(rest) == "" || indentOf(rest) >= f.indent || f.closedBy(line)
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
