---
status: accepted
date: 2026-11-04
amends: [57]
---

# 0066. 片付けの印を付ける先・外す先のメッセージがない（`message_not_found`）ときは、失敗にしない

## Context

片付けで消したセッションのスレッドの最初のメッセージに、Bot は `:broom:` を付ける（ADR 0057）。付けるときは
`already_reacted` だけを、外すときは `no_reaction` だけを失敗にせず、ほかは付けるときは片付けの失敗として返し、
外すときはログに書いていた。人がスレッドの最初のメッセージを消していると、Slack は `message_not_found` を返し、
片付けは済んでいるのに `cleaning up unused sessions failed` の ERROR のログが出た（#392）。印は、人がチャンネルの
一覧で最初のメッセージから読み取るためのもので、そのメッセージがなければ読み取る所もない。

## Decision

- Bot の `MarkCleaned` と、印を外す `unmarkCleaned` は、Slack の `message_not_found` を失敗にしない。付ける先も
  外す先もなく、することが残らないため。
- `channel_not_found` は失敗のままにする。チャンネルが消えたときだけでなく、Bot が見えないチャンネル（抜けた
  プライベートチャンネルなど）でも返り、直すべき設定のことがあるため。
- 片付けの使い方（`SessionCleaner` が印の失敗を `mark removal` として返すこと）は変えない。

## Alternatives

- **`SessionCleaner` で見分ける**（Bot が「付ける先がない」を表す決まったエラーを返し、片付けが捨てる）：Slack の
  答えをどう読むかは Bot の関心で、付ける側（`already_reacted`）と同じ所で捨てれば足りる。ポートに Slack 以外の
  知らせ先が使わないエラーを足すことになる。
- **印の失敗をすべて ERROR より低いレベルで書く**：トークンやスコープの誤りなど、直すべき失敗も目立たなくなる。
- **`channel_not_found` や `is_archived` も失敗にしない**：`channel_not_found` は上のとおり設定の誤りと見分けられない。
  アーカイブしたチャンネルでの片付けはまれで、起きたら別に決める。

## Consequences

- 最初のメッセージを消したスレッドのセッションを片付けても、ERROR のログは出ない。そのスレッドには印が付かない。
- 片付けた知らせ（スレッドへの投稿）は今のまま出す。最初のメッセージがないスレッドでの知らせの出方は、この判断の
  外にある。
