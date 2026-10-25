# slack の全体像

## 構成

```
slack/
  bin/slackctl     Slack の Web API と incoming webhook のクライアント
  scripts/check-markdown  Slack の markdown ブロックが違って読む形を見つける（slackctl check と post が使う）
  skills/slack/    読み方と書き方の手順
  tests/slackctl-test.sh  slackctl の振る舞いの検査（make check-platform が回す）
```

- `slackctl` は、渡された認証情報の組み合わせで、できること（読む、どこへ投稿するか）を決める。エージェントは
  `slackctl status` でそれを確かめてから使う。
- 認証情報の選び方と制約は `slackctl` にまとめ、スキルは何ができるかに応じた使い方だけを書く。

## 構成要素

### `bin/slackctl`

- **責務**：メッセージ、スレッド、リンク先を読み、標準の Markdown で投稿する。
- **置くもの / 置かないもの**：
  - 読んだメッセージの文は、Web API が置き換えた `&amp;`、`&lt;`、`&gt;` を元の文字に戻して出す（Slack の画面と同じ文字）。
    `<@U…>` などの Slack の記法はそのまま出すので、出力では、利用者が文字として打った `<@U…>` や `<https://…|表示>` と
    Slack の記法とを見分けられない。
  - Bot のトークンは、`SLACK_BOT_TOKEN_FILE` が読めればそのファイルから、呼ばれるたびに読む（毎回読むことは
    ハーネスの契約による）。なければ `SLACK_BOT_TOKEN` を使う。Bot のトークンがあれば、読むこともチャンネルとスレッドへの
    投稿もできる。
  - Bot のトークンがなく `SLACK_WEBHOOK_URL` があれば、投稿だけを incoming webhook で行う。チャンネルは webhook で
    決まり、スレッドには投稿できない。
  - control-plane が渡すのは `SLACK_BOT_TOKEN_FILE` だけ（[ハーネスの契約](../../../docs/reference/harness.md)）。`SLACK_BOT_TOKEN` と
    `SLACK_WEBHOOK_URL` は、人が対話で使うときに自分で用意する代わりの手段。
  - 投稿の前に、空でないことと、Slack の 1 件の上限（12,000 文字）に収まることを確かめる（`slackctl check`、`post` も
    同じ検査をする）。
  - あわせて、Slack で崩れる形を `scripts/check-markdown` で見つけ、行ごとに知らせる。`check` は見つかれば失敗し、`post` は
    見つかれば投稿せず、`--force` を付けたときだけそのまま投稿する。直しはせず、書き直すのはエージェント
    （[ADR 0028](../../../docs/adr/0028-slackctl-finds-forms-slack-reads-differently.md)）。
- **関係**：`curl`、`jq` を使い、崩れる形は `scripts/check-markdown` に任せる。振る舞いは `tests/slackctl-test.sh`（偽の `curl`）で確かめる。
- **当てはめた原則・パターン**：[使う側に届く契約](../../principles/docs/reference/patterns.md#published-language)：control-plane からの渡され方は
  契約に合わせる。[仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)：上限を超える投稿と、崩れる形の投稿は送る前に止める。

### `scripts/check-markdown`

- **責務**：標準の Markdown のうち、Slack の markdown ブロックが違って読む形（[Slack の markdown ブロックが読む形](reference/markdown.md)）を
  見つける。
- **置くもの / 置かないもの**：
  - 見つける形：裸の URL、太字の内側の ASCII の記号、すぐ上が文字の行の表、番号付きのリストの項目の下の入れ子、
    見出しの中の書式、リストの中のコードブロック。コードの中は読まない。
  - 見つけない形：実体参照（`&amp;` のようにわざと見せることがあり、写す元は `slackctl` が読むときに戻して断つ）、
    リストの項目の中の段落、`_…_` の斜体、引用の後の空行、`*…*` の斜体と `~~…~~` の打ち消しの内側の記号（
    確かめるのは `**` だけ）。
  - 行ごとに読む。太字の `**` は行の中で順に開きと閉じとして組にするので、組がずれると知らせることも見逃すこともある。
  - 標準入力の Markdown を読み、見つけた形を `line N: …` で出して終了コード 3 で終わる。なければ 0。ほかの終了コード
    （捕まえていない例外の 1 など）は検知の失敗で、`slackctl` は崩れる形とは分けて落とす。
- **関係**：`python3` を使い、`slackctl` の `check` と `post` から呼ばれる。振る舞いは `tests/slackctl-test.sh` で確かめる。

### `skills/slack/`

- **責務**：依頼が Slack を指すときや、Slack に投稿するときの手順。
- **置くもの / 置かないもの**：コマンドの使い方と、投稿してよい場面（ハーネスの契約の「Slack への投稿」の要約）と、
  書き方（標準の Markdown、Slack で崩れない形、上限を超えるとき）。崩れない形は[Slack の markdown ブロックが読む形](reference/markdown.md "在りか")の要約にする。認証情報の選び方は置かない。
- **関係**：control-plane 経由の Slack からの依頼では、プロンプトが依頼の場所だけを指すので、このスキルで本文と
  スレッドを読む（[ハーネスの契約](../../../docs/reference/harness.md)）。
