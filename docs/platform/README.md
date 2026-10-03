# platform

`platform/` は、ターゲットをまたいで使うエージェントの能力を Claude Code のプラグインとしてまとめたもの。
将来 agent-platform として別のリポジトリに切り出す（[ADR 0001](adr/0001-platform-as-plugin-marketplace.md)）。

## 構成

```
platform/
  .claude-plugin/marketplace.json   マーケットプレイス acp-platform
  plugins/
    roles/                          共通のロール（agents/<ロール名>.md）
    slack/                          Slack を読み書きする slackctl コマンド（bin/）と、その使い方のスキル
```

## 使われ方

- ターゲットは `.claude/settings.json` で使うプラグインを宣言する（[target ADR 0004](../target/adr/0004-plugins-declared-in-settings.md)）。
- control-plane から起動するエージェントには、control-plane が宣言を読んで `--plugin-dir` で読み込ませる。
- 人間が対話で使うときは、最初に一度マーケットプレイスを登録してプラグインを入れる。

  ```bash
  claude plugin marketplace add ./platform
  claude plugin install roles@acp-platform
  ```

- プラグインが使う認証情報は、実行する環境が環境変数で渡す。slack プラグインは `SLACK_BOT_TOKEN_FILE`
  （呼ばれるたびに読む）か `SLACK_BOT_TOKEN`、なければ投稿だけを `SLACK_WEBHOOK_URL` で行う。
- プラグインを変えたら `claude plugin validate ./platform` で検査し、`bin/` のスクリプトは `shellcheck` を通す。
- プラグインには、どのターゲットでも通用する内容だけを書く。
