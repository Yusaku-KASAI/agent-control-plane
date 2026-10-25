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
		{"link text starting with a url", "[https://x.example](https://y.example) と https://z.example", "[https://x.example](https://y.example) と <https://z.example>"},
		{"bracketed url", "[https://x.example/a] と https://z.example", "[<https://x.example/a>] と <https://z.example>"},
		{"url in brackets that are not a link", "[注：https://x.example]", "[注：<https://x.example>]"},
		{"code with ] in a link text", "[注 `]` https://a.example](https://b.example)", "[注 `]` https://a.example](https://b.example)"},
		{"brackets in a query", "https://x.example/?a[]=1&a[]=2 と 検索 https://x.example/?filter[state]=open の結果", "<https://x.example/?a[]=1&a[]=2> と 検索 <https://x.example/?filter[state]=open> の結果"},
		{"brackets in a query within brackets", "配列 a[i の https://x.example/?a[]=1 の結果 [注：https://x.example/?filter[state]=open]", "配列 a[i の <https://x.example/?a[]=1> の結果 [注：<https://x.example/?filter[state]=open>]"},
		{"reference link", "[https://x.example][1] と https://z.example", "[https://x.example][1] と <https://z.example>"},
		{"unclosed bracket before a closed one", "配列 a[i の https://a.example/x と [注]", "配列 a[i の <https://a.example/x> と [注]"},
		{"unclosed bracket", "配列 a[i の要素は https://example.com/a、後", "配列 a[i の要素は <https://example.com/a>、後"},
		{"non-ASCII path", "https://ja.wikipedia.org/wiki/日本 の記事", "<https://ja.wikipedia.org/wiki/>日本 の記事"},
		{"code span", "`https://example.com` と ``a ` https://example.com``", "`https://example.com` と ``a ` https://example.com``"},
		{"unclosed backtick", "` https://example.com", "` <https://example.com>"},
		{
			"fenced code",
			"前\n```\nhttps://example.com\n本文\n| a | b |\n| --- | --- |\n```\nhttps://example.com",
			"前\n```\nhttps://example.com\n本文\n| a | b |\n| --- | --- |\n```\n<https://example.com>",
		},
		{"fence in a quote", "> ```\n> https://x.example\n> ```\n> https://y.example", "> ```\n> https://x.example\n> ```\n> <https://y.example>"},
		{"fence on a list marker", "- ```bash\n  echo https://a.example\n  ```\n本文 https://b.example", "- ```bash\n  echo https://a.example\n  ```\n本文 <https://b.example>"},
		{"fence on a numbered list marker", "1. ```\n   https://a.example\n   ```\nhttps://b.example", "1. ```\n   https://a.example\n   ```\n<https://b.example>"},
		{"list marker fence inside a fence", "```markdown\n- ```\nhttps://in.example\n```\nhttps://out.example", "```markdown\n- ```\nhttps://in.example\n```\n<https://out.example>"},
		{"tilde fence in a list", "- 項目\n  ~~~\n  https://example.com\n  ~~~", "- 項目\n  ~~~\n  https://example.com\n  ~~~"},
		{"table under text", "表です\n| a | b |\n| --- | :-: |\n| 1 | 2 |", "表です\n\n| a | b |\n| --- | :-: |\n| 1 | 2 |"},
		{"table without outer bars", "表です\na | b\n--- | ---", "表です\n\na | b\n--- | ---"},
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
