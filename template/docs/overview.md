# ターゲットの環境の全体像

ターゲットは、人の開発環境とエージェントの実行環境を兼ねる devcontainer と、Claude Code の設定、検査を持つ
リポジトリ。control-plane は、ターゲットのイメージでエージェントのコンテナを作り、ターゲットが宣言したプラグインを
読み込ませる。

| 部分 | 中身 | 文書 |
| --- | --- | --- |
| devcontainer | `.devcontainer/`（イメージ、compose、VS Code の配線、ホストの準備） | [architecture/devcontainer.md](architecture/devcontainer.md) |
| Claude Code の設定 | `.claude/settings.json`（プラグインの宣言、対話のときの権限）、`CLAUDE.md` | [architecture/devcontainer.md](architecture/devcontainer.md) |
| 検査 | `Makefile` の `check-<関心>`、`.github/workflows/ci.yml`、`tools/check-docs.py` | [architecture/ci.md](architecture/ci.md) |
| 契約 | ターゲットが用意するものと、ファイルの分類（共通 / 埋める / 固有） | [reference/contract.md](reference/contract.md) |

control-plane がコンテナに渡すものは、platform の[ハーネスの契約](../../platform/docs/reference/harness.md)にある。
