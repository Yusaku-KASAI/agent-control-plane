---
status: accepted
date: 2026-10-04
amended_by: [11]
---

# 0005. CI は devcontainer のイメージの中で make check を実行する

## Context

main への変更は PR 経由にし、PR ごとに `make check`（lint とテスト）を CI で確かめる。ツールチェーンの
バージョンは devcontainer の Dockerfile で固定していて、エージェントと人間はそのイメージの中で `make check` を
実行している。

## Decision

- CI は devcontainer の Dockerfile からイメージをビルドし、その中で `node` として `make check` を実行する。
- ビルドは GitHub Actions のキャッシュを使う。

## Alternatives

- **actions/setup-go と golangci-lint-action を使う**：速いが、Go と golangci-lint のバージョンを Dockerfile と
  ワークフローの二か所に書くことになり、手元と CI の結果がずれうる。

## Consequences

- 手元、エージェント、CI が同じ環境で検査する。ツールのバージョンを上げるときは Dockerfile だけを変える。
- devcontainer のイメージがビルドできることも PR ごとに確かめられる。
- キャッシュが効かないときは、イメージのビルドに数分かかる。
