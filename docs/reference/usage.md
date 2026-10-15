# 依頼の仕方

エージェントへの依頼の出し方と止め方。依頼がどう扱われるか（待つ順番、流し込み、停止で止まるもの、再起動での中断）と
何がスレッドに返るかは[依頼と知らせ](../architecture/app/requests.md "在りか")に、セッションの動きは
[セッション](../architecture/app/sessions.md "在りか")にある。

## Slack から

| すること | 操作 |
| --- | --- |
| 依頼する | エージェントの Bot をメンションする。結果はそのスレッドに返る |
| 同じセッションに続けて依頼する | 同じスレッドの中でメンションする |
| 実行中のターンに流し込む | 本文を `:zap:` で始めてメンションする |
| 止める | ターンの間にスレッドに出る状態メッセージに `:no_entry_sign:` を付ける |

## CLI から

```bash
acp run --agent <name> [--session <id>] [--inject] [--server <url>] "<指示>"
acp stop --agent <name> --session <id> [--server <url>]
```

- サーバーの URL は `--server`、`ACP_SERVER`、`http://127.0.0.1:8787` の順で決まる。API トークンは
  `ACP_API_TOKEN` から読む。
- `--session` を省くと新しいセッションを始め、`--session <id>` を付けるとそのセッションの続きを依頼する。
  結果を stdout に、セッション ID と作業領域のパスを stderr に表示する。エージェントがエラー（使用上限を含む）で
  ターンを終えたときは、そう stderr に書き、終了コード 1 で終わる。
- `--inject` を付けると、実行中のターンに流し込む。`acp stop` は、Slack の `:no_entry_sign:` と同じくセッションを止める。
