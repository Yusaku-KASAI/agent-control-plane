package slackbot

import "testing"

func TestSlackMarkdown(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"url after Japanese", "PR：https://github.com/o/r/pull/1", "PR：<https://github.com/o/r/pull/1>"},
		{"url before Japanese", "https://example.com/a、後", "<https://example.com/a>、後"},
		{"url in full-width parentheses", "（https://example.com/a）", "（<https://example.com/a>）"},
		{"url in ASCII parentheses", "(https://example.com/a) と", "(<https://example.com/a>) と"},
		{"parentheses inside a url", "https://example.com/a_(b) と", "<https://example.com/a_(b)> と"},
		{"sentence end", "see https://example.com/a.", "see <https://example.com/a>."},
		{"query", "https://example.com/a?b=1&c=2#x", "<https://example.com/a?b=1&c=2#x>"},
		{"http", "http://example.com", "<http://example.com>"},
		{"bold url", "**https://example.com**", "**<https://example.com>**"},
		{"two urls", "https://a.example と https://b.example", "<https://a.example> と <https://b.example>"},
		{"scheme only", "https:// と", "https:// と"},
		{"autolink", "<https://example.com/a>（後", "<https://example.com/a>（後"},
		{"link destination", "[PR](https://example.com/a)後", "[PR](https://example.com/a)後"},
		{"link text", "[https://example.com](https://example.com)", "[https://example.com](https://example.com)"},
		{"url within a link text", "[詳細は https://x.example を](https://y.example) と https://z.example", "[詳細は https://x.example を](https://y.example) と <https://z.example>"},
		{"url inside an autolink", "<https://a.example/x?u=https://b.example>", "<https://a.example/x?u=https://b.example>"},
		{"url inside a link destination", "[PR](https://a.example/?next=https://b.example)", "[PR](https://a.example/?next=https://b.example)"},
		{"url inside a bare url", "https://a.example/?next=https://b.example", "<https://a.example/?next=https://b.example>"},
		{"unclosed bracket", "配列 a[i の要素は https://example.com/a、後", "配列 a[i の要素は <https://example.com/a>、後"},
		{"non-ASCII path", "https://ja.wikipedia.org/wiki/日本 の記事", "<https://ja.wikipedia.org/wiki/>日本 の記事"},
		{"code span", "`https://example.com` と ``a ` https://example.com``", "`https://example.com` と ``a ` https://example.com``"},
		{"unclosed backtick", "` https://example.com", "` <https://example.com>"},
		{
			"fenced code",
			"前\n```\nhttps://example.com\n本文\n| a | b |\n| --- | --- |\n```\nhttps://example.com",
			"前\n```\nhttps://example.com\n本文\n| a | b |\n| --- | --- |\n```\n<https://example.com>",
		},
		{"tilde fence in a list", "- 項目\n  ~~~\n  https://example.com\n  ~~~", "- 項目\n  ~~~\n  https://example.com\n  ~~~"},
		{"table under text", "表です\n| a | b |\n| --- | :-: |\n| 1 | 2 |", "表です\n\n| a | b |\n| --- | :-: |\n| 1 | 2 |"},
		{"table under a list", "- 項目\n| a | b |\n|---|---|", "- 項目\n\n| a | b |\n|---|---|"},
		{"table after a blank line", "表です\n\n| a | b |\n| --- | --- |", "表です\n\n| a | b |\n| --- | --- |"},
		{"table at the start", "| a | b |\n| --- | --- |\n| 1 | 2 |", "| a | b |\n| --- | --- |\n| 1 | 2 |"},
		{"rows of a table", "| a | b |\n| --- | --- |\n| 1 | 2 |\n| 3 | 4 |", "| a | b |\n| --- | --- |\n| 1 | 2 |\n| 3 | 4 |"},
		{"row without a delimiter", "本文\n| a | b |\n本文", "本文\n| a | b |\n本文"},
		{"setext heading", "見出し\n---", "見出し\n---"},
		{"url in a table", "| https://example.com/a |\n| --- |", "| <https://example.com/a> |\n| --- |"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := slackMarkdown(tt.in); got != tt.want {
				t.Errorf("slackMarkdown(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}
