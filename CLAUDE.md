# agent-control-plane

Claude Code エージェントをコンテナで隔離して実行し、Slack などから呼び出すための制御基盤（Go）。
このリポジトリ自身もエージェントが開発するターゲットの一つであり、platform と target-template の抽出元になる。

## 使われ方

依頼者一人が、自分の手元（Docker Desktop）で `acp serve` を動かし、Slack と CLI（`acp run`）からエージェントに
依頼する。ほかの人には使わせていない。

## 公開と運用

公開も配布もしていない。手元での動かし方は [docs/guides/operations.md](docs/guides/operations.md "案内")。

## 守る互換

互換を守る相手がいない。理想の形に向けてなら、記録や設定の形式、API、コマンドを破壊的に変えてよい。今あるデータや
設定の扱いは ADR に書く。ただし契約（用語集、ハーネスの契約、ターゲットの契約）、platform（プラグイン）、
principles と conventions、target-template（共通のファイル）を変えるときは、それを使うほかのターゲットも直す。

## 参照先

- [docs/README.md](docs/README.md "案内") — control-plane の文書の一覧。platform と target-template の文書は
  [platform/docs/README.md](platform/docs/README.md "案内")、[template/docs/README.md](template/docs/README.md "案内")
- [docs/overview.md](docs/overview.md "案内") — 狙い、考え方、全体の構成と地図、目指す姿

## 作業の完了条件

変更後は `make check`（関心ごとの検査をすべて）を通す。

## 言語

ドキュメントとコミットメッセージは日本語、コード中のコメントは英語。

## 共通のルール

コメントとドキュメントの書き方など、どのターゲットでも同じルールは、`.claude/settings.json` で有効にしている
conventions プラグイン（acp-platform マーケットプレイスで配る）が、セッションとサブエージェントの始めに読み込ませる。

## 他のリポジトリ

`/workspace` 配下の他のリポジトリは読み取り専用の参照。やり方や構造は参考にしてよいが、
固有の名前・プロダクト・業務知識はこのリポジトリに持ち込まない。
