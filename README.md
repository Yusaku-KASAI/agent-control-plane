# agent-control-plane

Claude Code エージェントをコンテナで隔離して実行する制御基盤。依頼を Slack と HTTP API で受けるサーバー（`acp serve`）と、
クライアント（`acp run`）からなる。

- 狙いと全体の構成：[docs/overview.md](docs/overview.md)
- 依頼の仕方：[docs/reference/usage.md](docs/reference/usage.md "案内")
- サーバーの準備と起動：[docs/guides/operations.md](docs/guides/operations.md "案内")
- 文書の一覧：[docs/README.md](docs/README.md)
