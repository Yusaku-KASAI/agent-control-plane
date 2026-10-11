# agent-control-plane

Claude Code エージェントをコンテナで隔離して実行する制御基盤。

- `acp serve`：依頼を Slack と HTTP API で受け、エージェントのコンテナを動かすサーバー。`compose.yaml` でコンテナとして
  動かす。Docker へは、使う API だけを通すプロキシ経由でアクセスする。
- `acp run`：サーバーに依頼して結果を表示するクライアント。

## 文書

| 文書 | 何があるか |
| --- | --- |
| [docs/overview.md](docs/overview.md) | 狙い、考え方、全体の構成、目指す姿 |
| [docs/app/usage.md](docs/app/usage.md) | 依頼の仕方（Slack、CLI） |
| [docs/app/operations.md](docs/app/operations.md) | サーバーの準備と起動、Slack のトークンの取り直し、クライアントのビルド |
| [docs/app/README.md](docs/app/README.md) | セッションのライフサイクル |
| [platform/docs/README.md](platform/docs/README.md) | エージェントの共通の能力（プラグイン）と、対話で使うときの準備 |
| [docs/README.md](docs/README.md) | 文書の構成と、ほかの文書の一覧 |
