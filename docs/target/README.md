# ターゲット

## イメージに求めること

control-plane はターゲットのイメージから使い捨てのコンテナを作り、`claude -p` を 1 回実行する。
そのためにイメージは次を満たす必要がある。

- entrypoint が root で動き、準備を済ませたあと引数のコマンドを `exec` する。
- エージェントを動かすユーザー `node` と、`claude`、`runuser` が入っている。control-plane は
  `runuser -u node -- claude ...` を実行する（Claude Code は root では権限確認を省略できないため）。
- entrypoint は環境変数 `WORKSPACE_DIR`（リポジトリのマウント先）と `FIREWALL_ENABLED` を解釈する。
- entrypoint 自身の出力は stderr に出す。stdout はエージェントの回答として扱われる。

このリポジトリの [devcontainer](../../.devcontainer/README.md) のイメージはこれを満たす。
