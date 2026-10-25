package slackbot

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// The cases are the slack plugin's: the forms Slack reads differently, which its check-markdown
// finds. slackMarkdown is tried on each, and what it gives is checked by check-markdown too, so
// that the two read Markdown alike (docs/adr/app/0051).
const (
	markdownCases = "../../../platform/plugins/slack/tests/markdown-cases.json"
	checkMarkdown = "../../../platform/plugins/slack/scripts/check-markdown"
)

// fixedMarkdown gives, by the name of a case, the text slackMarkdown makes of it. A case not here
// is left as it is.
var fixedMarkdown = map[string]string{
	"url after Japanese":                   "PR：<https://github.com/o/r/pull/1>",
	"url before Japanese":                  "<https://example.com/a>、後",
	"url in full-width parentheses":        "（<https://example.com/a>）",
	"url in ASCII parentheses":             "(<https://example.com/a>) と",
	"parentheses inside a url":             "<https://example.com/a_(b)> と",
	"sentence end":                         "see <https://example.com/a>.",
	"query":                                "<https://example.com/a?b=1&c=2#x>",
	"http":                                 "<http://example.com>",
	"bold url":                             "**<https://example.com>**",
	"two urls":                             "<https://a.example> と <https://b.example>",
	"url within a link text":               "[詳細は https://x.example を](https://y.example) と <https://z.example>",
	"url inside a bare url":                "<https://a.example/?next=https://b.example>",
	"link text starting with a url":        "[https://x.example](https://y.example) と <https://z.example>",
	"bracketed url":                        "[<https://x.example/a>] と <https://z.example>",
	"url in brackets that are not a link":  "[注：<https://x.example>]",
	"brackets in a query":                  "<https://x.example/?a[]=1&a[]=2> と 検索 <https://x.example/?filter[state]=open> の結果",
	"brackets in a query within brackets":  "配列 a[i の <https://x.example/?a[]=1> の結果 [注：<https://x.example/?filter[state]=open>]",
	"reference link":                       "[https://x.example][1] と <https://z.example>",
	"unclosed bracket before a closed one": "配列 a[i の <https://a.example/x> と [注]",
	"unclosed bracket":                     "配列 a[i の要素は <https://example.com/a>、後",
	"non-ASCII path":                       "<https://ja.wikipedia.org/wiki/>日本 の記事",
	"unclosed backtick":                    "` <https://example.com>",
	"fenced code":                          "前\n```\nhttps://example.com\n本文\n| a | b |\n| --- | --- |\n```\n<https://example.com>",
	"fence in a quote":                     "> ```\n> https://x.example\n> ```\n> <https://y.example>",
	"fence on a list marker":               "- ```bash\n  echo https://a.example\n  ```\n本文 <https://b.example>",
	"fence on a numbered list marker":      "1. ```\n   https://a.example\n   ```\n<https://b.example>",
	"list marker fence inside a fence":     "```markdown\n- ```\nhttps://in.example\n```\n<https://out.example>",
	"table under text":                     "表です\n\n| a | b |\n| --- | :-: |\n| 1 | 2 |",
	"table without outer bars":             "表です\n\na | b\n--- | ---",
	"table under a list":                   "- 項目\n\n| a | b |\n|---|---|",
	"url in a table":                       "| <https://example.com/a> |\n| --- |",
	"a bare URL after Japanese is found":   "PR：<https://x.example/pull/1>",
	"a table right under text is found":    "表です\n\n| a | b |\n| --- | --- |",
	"the text after a code block is read":  "```\ncode\n```\nPR：<https://x.example>",
	"inline code at the start of a line is no code block":               "```x``` は先頭\nPR：<https://x.example>",
	"a code block left open in a list item ends with the item":          "- 一\n  ```\n  code\n- 二：<https://a.example>",
	"a code block left open in a list item, before an item opening one": "- 一\n  ```\n  code\n- ```\n  https://in.example\n  ```\n本文：<https://out.example>",
	"a code block in a quote ends with the quote":                       "> ```\n> https://in.example\n\n本文：<https://out.example>",
	"a code block in a quote ends at a line without the mark":           "> ```\n> https://in.example\n本文：<https://out.example>",
	"a code block in a nested quote is closed there":                    "> > ```\n> > https://in.example\n> > ```\n> > 後：<https://out.example>",
	"quote marks in a code block are code":                              "```\n> https://in.example\n```\n後：<https://out.example>",
	"an indented code block outside a list holds lines indented less":   "   ```\ncurl https://in.example/a\n   ```\n後：<https://out.example>",
	"a code block indented deeper than its item holds the item's lines": "- 項目\n    ```\n  curl https://in.example/a\n    ```\n  続き：<https://out.example>",
	"a code block of an outer item ends with that item":                 "- a\n  - b\n  ```\n  https://in.example\n- c：<https://out.example>",
	"a tab indents code in a list item as far as the item's content":    "- a\n  ```\n\tcurl https://in.example\n  ```\n後：<https://out.example>",
	"a code block in a quote in a nested list item":                     "- a\n  - b\n    > ```\n    > https://in.example\n    > ```\n後：<https://out.example>",
	"a code block in a quote in an item past the ninth":                 "10. a\n    > ```\n    > PR：https://in.example\n    > ```\n後：<https://out.example>",
}

// formsFixed are check-markdown's findings of the forms slackMarkdown fixes, told by how their
// messages start. Each start must be found in some case before fixing, or the check would pass on
// messages it no longer tells apart.
var (
	formsFixed      = regexp.MustCompile(`(?m)^line \d+: (URL を|表の前に).*$`)
	formsFixedStart = []string{"URL を", "表の前に"}
)

func TestSlackMarkdown(t *testing.T) {
	b, err := os.ReadFile(markdownCases)
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name string `json:"name"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	named := map[string]bool{}
	before := ""
	for _, tt := range cases {
		named[tt.Name] = true
		before += runCheckMarkdown(t, tt.Text)
		t.Run(tt.Name, func(t *testing.T) {
			want, ok := fixedMarkdown[tt.Name]
			if !ok {
				want = tt.Text
			}
			got := slackMarkdown(tt.Text)
			if got != want {
				t.Errorf("slackMarkdown(%q)\n got %q\nwant %q", tt.Text, got, want)
			}
			if left := formsFixed.FindAllString(runCheckMarkdown(t, got), -1); len(left) > 0 {
				t.Errorf("check-markdown still finds in %q:\n%s", got, strings.Join(left, "\n"))
			}
		})
	}
	for _, start := range formsFixedStart {
		if !strings.Contains(before, ": "+start) {
			t.Errorf("check-markdown finds no %q in any case; formsFixed is out of date", start)
		}
	}
	for name := range fixedMarkdown {
		if !named[name] {
			t.Errorf("no case %q in %s", name, markdownCases)
		}
	}
}

func runCheckMarkdown(t *testing.T, text string) string {
	t.Helper()
	cmd := exec.Command("python3", "-I", checkMarkdown)
	cmd.Stdin = strings.NewReader(text)
	out, err := cmd.Output()
	// Exit status 3 tells there are findings; any other failure is a failure of the check.
	if exitErr, ok := err.(*exec.ExitError); err != nil && (!ok || exitErr.ExitCode() != 3) {
		t.Fatalf("check-markdown: %v", err)
	}
	return string(out)
}
