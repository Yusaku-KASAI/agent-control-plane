---
status: accepted
date: 2026-09-30
amended_by: [23]
---

# 0007. Claude Code の認証方式を差し替え可能にする

## Context

Claude Code の認証には、ブラウザでの OAuth ログイン、OAuth トークン（`claude setup-token`）、API キーがある。
当面は手元の PC で動かすが、将来は別の環境へ移す可能性がある。

## Decision

認証方式を差し替えられる形にする。最初は手元と同じブラウザでの OAuth ログインを使う。

## Alternatives

- **一つの方式に固定する**：実行環境を移すときに作り直しが必要になる。

## Consequences

- control-plane は、選んだ方式に応じて認証情報をエージェントのコンテナへ渡す仕組みを持つ。
