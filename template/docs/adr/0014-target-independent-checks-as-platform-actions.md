---
status: accepted
date: 2026-10-18
amends: [5, 11]
amended_by: [16]
---

# 0014. ターゲットに依存しない検査は、platform の複合アクションとして devcontainer の外で CI に回す

## Context

ADR 0005 で、CI は devcontainer のイメージの中で `make check` を実行すると決めた。ツールチェーンのバージョンを
Dockerfile の一か所で固め、手元と CI の結果をそろえるためである。ADR 0011 で、CI のジョブは関心ごとに、同じ名前の
Dockerfile のステージをビルドすると決めた。

文書の検査は conventions プラグインのコマンドになり、ターゲットの共通のファイル（`tools/check-docs.py`）と
`make check` から外れた（conventions の ADR 0019）。この検査が使うのは `python3` と `git` だけで、ターゲットの
ツールチェーンに依存しない。それでも CI で回さないと、人が Claude Code を通さずに変えた文書が確かめられない。

## Decision

- ターゲットのツールチェーンに依存しない検査は、platform が GitHub Actions の複合アクションとして配り、ターゲットの
  CI はそれを devcontainer のイメージの外で呼んでよい。今は文書の検査だけ
  （`platform/plugins/conventions/actions/check-docs`）。
- ターゲットの `ci.yml` は、`check-docs` ジョブで `uses: Yusaku-KASAI/agent-control-plane/platform/plugins/conventions/actions/check-docs@main`
  の一行を書き、`check` ジョブはそのジョブも待つ。このジョブは Dockerfile のステージを持たない。版は `main` にする
  （内部でだけ使うため）。ほかのリポジトリのコードを動かすので、ジョブの権限は `contents: read` だけにし、アクションは
  チェックアウトの認証情報を残さない。

## Alternatives

- **devcontainer のイメージの中で、platform を取ってきて `make` から回す**：Dockerfile のステージと Makefile の
  ルールに、platform の在りかと取り方を書くことになる。前の `tools/check-docs.py` と同じく、検査の具体がターゲットの
  共通のファイルに出る。
- **再利用できるワークフロー（`workflow_call`）**：ジョブの定義（ランナーなど）まで platform に置ける。ただし、呼ぶ
  側の `check` ジョブの待ち方は結局ターゲットに書く。手順のまとまりを借りるだけなら、複合アクションで足りる。
- **アクションの版を SHA に固める**：CI の結果が platform の変更で動かなくなるが、決まりを変えるたびに、すべての
  ターゲットで版を上げることになる。
- **ターゲットの CI では回さない**：エージェントのターンの終わりの hook だけでは、人が直接変えた文書を見逃す。

## Consequences

- ターゲットの CI の文書の検査は、platform の `main` の検査で回る。platform の決まりを変えると、ターゲットの
  文書がそれに合っていなければ、次の CI で落ちる。
- エージェントの手元の検査は、入れたマーケットプレイスの版で回るので、`main` と食い違うことがある。
- このアクションは devcontainer のイメージを使わないので、ランナーの `python3` の版で動く。
