# ターゲット

## イメージに求めること

control-plane はセッションごとにターゲットのイメージからコンテナを作り、`claude -p` を `stream-json` の
入出力で動かし続けて、依頼を標準入力に送る。そのためにイメージは次を満たす必要がある。

- entrypoint が root で動き、準備を済ませたあと引数のコマンドを `exec` する。標準入力はそのまま渡す。
- エージェントを動かすユーザー `node` と、`claude`、`runuser`、`git`、`gh` が入っている。control-plane は
  `runuser -u node -- claude ...` を実行する（Claude Code は root では権限確認を省略できないため）。
- entrypoint は環境変数 `WORKSPACE_DIR`（リポジトリのマウント先）と `FIREWALL_ENABLED` を解釈する。

control-plane はさらに次の環境変数を渡す。

| 変数 | 内容 |
| --- | --- |
| `ACP_SESSION_BRANCH` | セッションの既定の作業ブランチ |
| `GIT_AUTHOR_*` / `GIT_COMMITTER_*` | エージェントのコミットの作者（設定されている場合） |
| `GH_CONFIG_DIR` と `GIT_CONFIG_*` | GitHub のトークンを置いた `gh` の設定ディレクトリと、git がそれを `gh` 経由で使う設定（GitHub App がある場合） |

認証情報は `/run/acp/credentials` に読み取り専用でマウントされ、control-plane が失効前に書き換える。
- entrypoint 自身の出力は stderr に出す。stdout はエージェントの回答として扱われる。

## プラグインとロール

- ターゲットが使うプラグインは `.claude/settings.json` の `extraKnownMarketplaces` と `enabledPlugins` で宣言する
  （[ADR 0004](adr/0004-plugins-declared-in-settings.md)）。control-plane は、ターゲットの中にあるマーケットプレイス
  （`directory` で相対パスを指すもの）から有効なプラグインを `--plugin-dir` で読み込ませる。
- ロールは Claude Code のカスタムエージェントとして定義し、control-plane は `claude -p --agent <ロール名>` で
  起動する（[ADR 0003](adr/0003-roles-as-claude-code-agents.md)）。共有のロールは [platform](../platform/README.md) の
  `roles` プラグインにあり、ターゲットの `.claude/agents/<ロール名>.md` に同名の定義を置くとそちらが優先される。
- ロールの定義には振る舞いだけを書き、ターゲット固有の事実や規約は `CLAUDE.md` に書く。
- 定義が見つからないロールで起動すると、エージェントはエラーで終了する。
