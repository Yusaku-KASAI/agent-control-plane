# 0001. domain / usecase / adapter の 3 層に分け、依存の向きを機械的に検査する

- Status: Accepted
- Date: 2026-09-30

## Context

control-plane は Docker、Slack、設定ファイルなど外部との接点が多く、今後も差し替えや追加が続く
（app ADR 0007、0008）。外部の都合がアプリの中心の処理に入り込まないようにしたい。

## Decision

- `internal/domain`、`internal/usecase`、`internal/adapter/<技術>` の 3 層に分け、依存は
  adapter → usecase → domain の一方向にする。組み立ては `cmd/acp` で手書きする。
- usecase が使う外部とのインターフェースは usecase パッケージに置く。Go では使う側がインターフェースを
  定義するのが慣習であり、port 専用のパッケージは設けない。
- 依存の向きは golangci-lint の depguard で検査する。

## Alternatives

- **domain / application / infrastructure という命名**：分け方は同じで、名前が違うだけ。
- **機能ごとにパッケージを切る**：Go らしい形だが、層の依存の向きを機械的に検査しにくい。
- **DI コンテナを使う**：部品が少ないうちは手書きの組み立てで足りる。

## Consequences

- 外部ライブラリを使うコードは adapter にしか置けない。
- usecase のテストは、インターフェースの偽物だけで書ける。
