---
status: accepted
date: 2026-10-03
amends: [7, 9]
---

# 0023. エージェントの Claude Code は専用のトークンで認証し、設定ディレクトリをエージェントごとに分ける

## Context

エージェントのコンテナは、devcontainer でブラウザからログインした Claude Code の設定ボリュームを共有していた。
そこには認証情報のほか、devcontainer での会話の履歴や自動メモリも入っており、すべてのエージェントから読み書き
できた。`claude setup-token` は、ブラウザなしで使える長期の OAuth トークンを発行し、Claude Code は環境変数
`CLAUDE_CODE_OAUTH_TOKEN` でそれを使う。Claude Code の設定ディレクトリには、`--resume` に要る会話の履歴と
自動メモリが入る。

## Decision

- エージェントの Claude Code は、`claude setup-token` の OAuth トークンか API キーで認証する。control-plane は
  それをほかの認証情報と同じく発行して渡す。
- Claude Code の設定ディレクトリは、control-plane が管理するボリュームにエージェントごとに作る。devcontainer の
  設定ボリュームはエージェントに渡さない。

## Alternatives

- **ブラウザでのログイン情報をエージェントごとに写す**：更新トークンが別々に更新され、互いを無効にしうる。
- **設定ディレクトリをセッションごとに分ける**：自動メモリがセッションをまたいで引き継がれない。
- **共有を続ける**：devcontainer での会話や自動メモリがエージェントから読め、隔離（ADR 0003）に反する。

## Consequences

- 同じエージェントのセッションは自動メモリを共有し、エージェント同士と devcontainer とは分かれる。
- 共有していたボリュームにある既存のセッションの会話は引き継がれない。
- OAuth トークンは長期間有効なため、コンテナの環境変数として渡す。
