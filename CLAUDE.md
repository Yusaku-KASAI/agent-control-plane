# agent-control-plane

Claude Code エージェントをコンテナで隔離して実行し、Slack などから呼び出すための制御基盤（Go）。
このリポジトリ自身もエージェントが開発するターゲットの一つであり、target-template の抽出元になる。

## 参照先

- [docs/overview.md](docs/overview.md) — 狙い、考え方、全体の構成、目指す姿
- [docs/README.md](docs/README.md) — ドキュメントの構成、ADR、ドキュメントと Issue の分担
- [.devcontainer/README.md](.devcontainer/README.md) — 開発・エージェント実行環境
- [docs/codebase/README.md](docs/codebase/README.md) — Go コードの層と書き方
- [docs/target/README.md](docs/target/README.md#作業の進め方) — PR と Issue での作業の進め方

## 作業の完了条件

変更後は `make check`（lint とテスト）を通す。

## 言語

ドキュメントとコミットメッセージは日本語、コード中のコメントは英語。

## 共通のルール

コメントとドキュメントの書き方など、どのターゲットでも同じルールは platform の rules プラグイン
（[platform/plugins/rules/rules/](platform/plugins/rules/rules/)）が、セッションとサブエージェントの始めに読み込ませる。

## アプリの仕様の変更

control-plane は個人の手元だけで動き、互換を守る相手がいない。理想の形に向けてなら、記録や設定の形式、API、
コマンドを破壊的に変えてよい。今あるデータや設定の扱いは ADR に書く。

## 他のリポジトリ

`/workspace` 配下の他のリポジトリは読み取り専用の参照。やり方や構造は参考にしてよいが、
固有の名前・プロダクト・業務知識はこのリポジトリに持ち込まない。
