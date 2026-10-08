# platform

`platform/` は、ターゲットをまたいで使うエージェントの能力を Claude Code のプラグインとしてまとめたもの。
将来 agent-platform として別のリポジトリに切り出す（[ADR 0001](adr/0001-platform-as-plugin-marketplace.md)）。

## 構成

```
platform/
  .claude-plugin/marketplace.json   マーケットプレイス acp-platform
  plugins/
    roles/                          共通のロール（agents/<ロール名>.md）：developer（変更を実装する）、guide（質問に答える、変更しない）。
                                    ロールが使うモデルも定義で決める（ADR 0003）
    slack/                          Slack を読み書きする slackctl コマンド（bin/）と、その使い方のスキル
    github/                         PR の本文を今のブランチに合わせ続ける prctl コマンドと hook、GitHub での作業のスキル
    rules/                          どのターゲットでも同じルール（rules/*.md）。セッションとサブエージェントの始めに hook で読み込ませる（ADR 0004）
    bell/                           対話で使うとき、Claude Code が止まったり応答を待ったりしたら端末のベルを鳴らす hook
```

## 使われ方

- ターゲットは `.claude/settings.json` で使うプラグインを宣言する（[target ADR 0004](../target/adr/0004-plugins-declared-in-settings.md)）。
- control-plane から起動するエージェントには、control-plane が宣言を読んで `--plugin-dir` で読み込ませる。
- どのターゲットも `acp-platform` を、切り出した後の場所 `/workspace/agent-platform` で宣言する。切り出すまでは、
  ホストでリポジトリを並べたディレクトリに、相対パスのシンボリックリンクを置いてこの `platform/` を指す
  （`ln -s agent-control-plane/platform agent-platform`）。devcontainer でも、エージェントのコンテナでも、この
  リンクを通して届く。このリポジトリのエージェントは、リンクの先が自身の中なので、作業中のブランチの版を使う。
- 人間が対話で使うときは、最初に一度マーケットプレイスを登録してプラグインを入れる。

  ```bash
  claude plugin marketplace add /workspace/agent-platform
  claude plugin install roles@acp-platform
  ```

- プラグインが使う認証情報は、実行する環境が環境変数で渡す。slack プラグインは `SLACK_BOT_TOKEN_FILE`
  （呼ばれるたびに読む）か `SLACK_BOT_TOKEN`、なければ投稿だけを `SLACK_WEBHOOK_URL` で行う。
- github プラグインは `gh` の認証を使う（エージェントには control-plane が `GH_CONFIG_DIR` で渡す）。push の直後と
  ターンの終わりに hook が PR の本文が古くないかを確かめ、古ければ書き換えを促す
  （[ADR 0002](adr/0002-pr-body-tracks-head.md)）。
- プラグインの検査（`claude plugin validate ./platform` と `bin/` の `shellcheck`）は `make check-platform` で行う。
- プラグインには、どのターゲットでも通用する内容だけを書く。
