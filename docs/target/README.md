# ターゲット

## イメージに求めること

control-plane はターゲットのイメージから使い捨てのコンテナを作り、`claude -p` を 1 回実行する。
そのためにイメージは次を満たす必要がある。

- entrypoint が root で動き、準備を済ませたあと引数のコマンドを `exec` する。
- エージェントを動かすユーザー `node` と、`claude`、`runuser`、`git`、`gh` が入っている。control-plane は
  `runuser -u node -- claude ...` を実行する（Claude Code は root では権限確認を省略できないため）。
- entrypoint は環境変数 `WORKSPACE_DIR`（リポジトリのマウント先）と `FIREWALL_ENABLED` を解釈する。

control-plane はさらに次の環境変数を渡す。

| 変数 | 内容 |
| --- | --- |
| `ACP_SESSION_BRANCH` | セッションの既定の作業ブランチ |
| `GIT_AUTHOR_*` / `GIT_COMMITTER_*` | エージェントのコミットの作者（設定されている場合） |
| `GH_TOKEN` と `GIT_CONFIG_*` | GitHub のトークンと、git がそれを `gh` 経由で使う設定（GitHub App がある場合） |
- entrypoint 自身の出力は stderr に出す。stdout はエージェントの回答として扱われる。

## ロール

ロールは Claude Code のカスタムエージェントとして定義し、control-plane は `claude -p --agent <ロール名>` で
起動する（[ADR 0003](adr/0003-roles-as-claude-code-agents.md)）。

- 共有のロールはプラグインで提供され、ターゲットは `.claude/settings.json` で使うプラグインを宣言する。
  ターゲットの `.claude/agents/<ロール名>.md` に同名の定義を置くと、そちらが優先される。
- ロールの定義には振る舞いだけを書き、ターゲット固有の事実や規約は `CLAUDE.md` に書く。
- 定義が見つからないロールで起動すると、エージェントはエラーで終了する。
