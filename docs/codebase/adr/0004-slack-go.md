---
status: accepted
date: 2026-10-02
---

# 0004. Slack の API と Socket Mode には slack-go/slack を使う

## Context

Slack から依頼を受けて返信する（app ADR 0016）。Socket Mode には WebSocket の接続、再接続、イベントへの
応答が要る。

## Decision

`github.com/slack-go/slack` を使う。使うのは `internal/adapter/slackbot` の中だけにする。

## Alternatives

- **自前で実装する**：依存は減るが、再接続などの細かい処理を自分で書いて保守することになる。

## Consequences

- Slack の API の変更への追従は、このライブラリの更新に頼る。
