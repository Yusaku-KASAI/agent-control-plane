---
status: accepted
date: 2026-11-03
---

# 0019. 入口と文書の根のひな形は、名前の終わりに `.tmpl` を足して写す元に置く

## Context

写す元の `target/`（ADR 0018）には環境のファイルのひな形だけを置き、`CLAUDE.md`、`README.md`、ターゲット自身の
`docs/` の器は、新しいターゲットを作る手順で一から書かせていた。`target/` の下の `CLAUDE.md` は、agent-control-plane で
その下のファイルを読むエージェントに読み込まれ、`.md` で終わる文書は agent-control-plane の文書として `check-docs` に
検査され、ターゲット自身の文書の根の器は agent-control-plane の地図に載せる決まりが要るため。

## Decision

- `CLAUDE.md`、`README.md`、`docs/README.md`、`docs/overview.md` のひな形を、名前の終わりに `.tmpl` を足して `target/` に
  置く（`CLAUDE.md.tmpl` など）。写したら `.tmpl` を外す。
- 埋める所は、環境のファイルと同じく `Fill in:` の印（Markdown では HTML のコメント）と `<...>` で示す。
- 接尾辞は `.env.example` の `.example` と分ける。`.env.example` は写したあとも例として残し、`.tmpl` は写したら外す。
  分ければ、外すものを一律に見分けられる。

## Alternatives

- **`.example` を接尾辞にする**：`.env.example` と見分けがつかず、写したあとに名前を戻すものを手順で一つずつ挙げる
  ことになる。
- **終わりを `.md` のままにする（`CLAUDE.example.md`）**：agent-control-plane の `check-docs` に検査され、中のリンク
  （`docs/README.md` など）が名前を戻す前は切れる。
- **ひな形を置かず、手順で一から書かせる**：契約の節や索引の形を、ターゲットごとに書き写すことになる。

## Consequences

- `target/` を写して名前を戻したものに `check-docs` を回せば、入口からの届き方、リンク、文書の根の形を確かめられる。
  `CLAUDE.md.tmpl` の節がターゲットの契約の CLAUDE.md の節と合うかは、それでは確かめられない。
- `CLAUDE.md.tmpl` は、ターゲットの契約の CLAUDE.md の節を写す。契約が変わったら、ひな形も直す。
