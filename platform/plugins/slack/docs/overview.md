# slack の全体像

## 構成

```
slack/
  bin/slackctl     Slack の Web API と incoming webhook のクライアント
  skills/slack/    読み方と書き方の手順
```

- `slackctl` は、渡された認証情報の組み合わせで、できること（読む、どこへ投稿するか）を決める。エージェントは
  `slackctl status` でそれを確かめてから使う。
- 認証情報の選び方と制約は `slackctl` にまとめ、スキルは何ができるかに応じた使い方だけを書く。

## 構成要素

### `bin/slackctl`

- **責務**：メッセージ、スレッド、リンク先を読み、標準の Markdown で投稿する。
- **置くもの / 置かないもの**：
  - Bot のトークンは、`SLACK_BOT_TOKEN_FILE` が読めればそのファイルから、呼ばれるたびに読む（毎回読むことは
    ハーネスの契約による）。なければ `SLACK_BOT_TOKEN` を使う。Bot のトークンがあれば、読むこともチャンネルとスレッドへの
    投稿もできる。
  - Bot のトークンがなく `SLACK_WEBHOOK_URL` があれば、投稿だけを incoming webhook で行う。チャンネルは webhook で
    決まり、スレッドには投稿できない。
  - control-plane が渡すのは `SLACK_BOT_TOKEN_FILE` だけ（[ハーネスの契約](../../../docs/reference/harness.md)）。`SLACK_BOT_TOKEN` と
    `SLACK_WEBHOOK_URL` は、人が対話で使うときに自分で用意する代わりの手段。
  - 投稿の前に、空でないことと、Slack の 1 件の上限（12,000 文字）に収まることを確かめる（`slackctl check`、`post` も
    同じ検査をする）。
- **関係**：`curl`、`jq` を使う。
- **当てはめた原則・パターン**：[使う側に届く契約](../../principles/docs/reference/patterns.md#published-language)：control-plane からの渡され方は
  契約に合わせる。[仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)：上限を超える投稿は送る前に止める。

### `skills/slack/`

- **責務**：依頼が Slack を指すときや、Slack に投稿するときの手順。
- **置くもの / 置かないもの**：コマンドの使い方と、書き方（標準の Markdown、上限を超えるとき）。認証情報の選び方は
  置かない。
- **関係**：control-plane 経由の Slack からの依頼では、プロンプトが依頼の場所だけを指すので、このスキルで本文と
  スレッドを読む（[ハーネスの契約](../../../docs/reference/harness.md)）。
