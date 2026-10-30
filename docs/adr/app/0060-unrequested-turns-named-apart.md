---
status: accepted
date: 2026-10-30
amends: [36, 58]
---

# 0060. ターンの終わりの「依頼の入らなかったターン」を、始まりの「エージェントのターン」と別の名前で持つ

## Context

`SessionEvent.AgentStarted` は、`SessionTurnStarted` ではエージェントが自分で始めたターンか（ターンの始まり）を、
`SessionTurnFinished` では取り込まれた依頼がないまま終わったターンか（ターンの終わり）を表していた。二つは違いうる。
エージェントのターンが流し込んだ依頼を取り込めば依頼の入ったターンになり、依頼のターンが依頼を取り込む前に停止や
失敗で終われば依頼の入らなかったターンになる。使用量の記録の `TurnRecord.AgentStarted` と `usage_log` の
`agent_started` の中身も終わりの意味だった。文書では始まりと終わりを別の言葉で呼ぶ（`requests.md` のターンの呼び方）
ようにしたが、コードと記録の名前には二つの意味が残っていた（Issue #338）。

## Decision

- `SessionEvent` は、始まりの意味を `AgentStarted`（`SessionTurnStarted` だけに付ける）、終わりの意味を `Unrequested`
  （`SessionTurnFinished` だけに付ける）の別の項目で持つ。
- `TurnRecord` の項目は `Unrequested`、`usage_log` の行の項目は `unrequested` にする。中身は変えない。
- 今ある `usage_log` の行は書き換えない。変える前の行の `agent_started` は、変えたあとの `unrequested` と同じ中身として
  読む（両方の行を集計するなら `.unrequested // .agent_started`）。

## Alternatives

- **名前を残し、コメントで二つの意味を説明する**：読む側がイベントの種類ごとに意味を読み分けることになり、文書の
  呼び方とも合わない。
- **一つの項目を値の型（始まり方、終わり方の列挙）にする**：始まりと終わりでとる値が違い、どちらにも当たらない
  イベントでの値も要る。真偽の二つの項目の方が小さい。
- **`usage_log` の項目名は残す**：記録を読むプログラムはなく、人が `jq` で読む。互換を守る相手がない（CLAUDE.md の
  「守る互換」）ので、文書と同じ呼び方に合わせる。
- **今ある行を書き換えて項目名をそろえる**：手元の記録を変える手順が要り、失敗すると記録を失う。読むときに二つの名前を
  受ければ足りる。

## Consequences

- ADR 0036 の「エージェントが自分で始めたか」の項目は、依頼の入らなかったターンか（`unrequested`）と読む。ADR 0058 の
  `SessionEvent.AgentStarted`（ターンを依頼なしとして知らせるか）は、`SessionEvent.Unrequested` と読む。
- 一つの `usage_log` のファイルに、`agent_started` の行と `unrequested` の行が混ざりうる。
