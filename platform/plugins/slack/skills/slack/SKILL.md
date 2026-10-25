---
name: slack
description: Slack のメッセージやスレッドを読むとき、Slack のメッセージへのリンクを開くとき、Slack に投稿するときに使う。依頼が Slack のメッセージやスレッドを指しているときも使う。
---

# Slack を読む・書く

`slackctl` コマンドで Slack を読み書きする。まず `slackctl status` で、この環境で何ができるかを確かめる。

## 読む

```bash
slackctl message <channel> <ts>        # 1 件のメッセージ
slackctl thread <channel> <thread_ts>  # スレッドのすべてのメッセージ
slackctl link <permalink>              # リンク先のメッセージと、そのスレッド
```

- 出力の `<@U…>` はユーザー、`[1700000000.000100]` はメッセージの ts。
- 読めるのは Bot が参加しているチャンネルだけ。`not_in_channel` などで読めないときは、そう報告する。
- `status` が `read: none` のときは、Slack の MCP の道具が使えればそれで読む。使えなければ読めないと報告する。

## 書く

```bash
slackctl check < message.md                 # 投稿前の検査
slackctl post <channel> <thread_ts> < message.md
```

- 投稿するのは、依頼者に頼まれたときか、Slack に届けるほかの手段がないとき。control-plane 経由で動くとき
  （[ハーネスの契約](../../../../docs/reference/harness.md "在りか")の動き方）は、ターンの最後の回答を control-plane が依頼の
  スレッドに代わりに投稿するので、最後の回答は投稿しない。devcontainer で直接動くときは、最後の回答は端末に出るだけ
  なので、Slack に届けるなら投稿する。
- 本文は標準の Markdown で書く（`**太字**`、`` `code` ``、`- 箇条書き`、`[表示](URL)`）。Slack 独自の `*太字*` は使わない。
- Slack は Markdown を少し違う規則で読む。Slack に届く文章（自分の投稿と、Slack からの依頼への最後の回答）は、
  次のとおりに書く（[Slack の markdown ブロックが読む形](../../docs/reference/markdown.md)）。
  - URL は `<https://…>` か `[表示](https://…)` で書く。裸の URL は、前後に日本語が続くとリンクにならないか、後ろの
    文字までリンクに入る。
  - `**` の内側が ASCII の記号（`` ` ``、`#`、`(`、`<` など）で始まるか終わるときは、`**` の外側に空白を置く。
    ``、**`code`**：`` や `**#21 …**：` は太字にならない。
  - 番号付きのリストの項目の下に入れ子を置かない。後ろの項目の番号が 1 から振り直しになる。入れ子が要るときは、
    外側を箇条書きにするか、番号を太字の段落（`**1. 題**`）にしてその下にリストを置く。
  - 表の前には空行を置く。見出しの中に書式を入れない。コードブロックはリストの外に置き、リストの項目の中で段落を分けない。
  - `<`、`>`、`&` は実体参照（`&lt;` など）にせず、元の文字で書く。`slackctl` で読んだ文面の `&lt;` などは、元の文字に戻して写す。
- 1 件は 12,000 文字まで。超えるときは要約するか、分けて投稿する。
- `status` の post が incoming webhook のときは、チャンネルもスレッドも指定できない（`slackctl post < message.md`）。
