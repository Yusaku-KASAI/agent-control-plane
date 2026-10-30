---
status: accepted
date: 2026-10-30
amends: ["../../../../platform/plugins/conventions/docs/adr/0003-type-first-docs-and-platform-kernel.md"]
---

# 0018. target-template は写す元の `target/` に置き、その文書は `target/template/docs/` に置く

## Context

target-template は agent-control-plane に同居し、文書は最上位の `template/docs/` に置き（conventions の ADR 0003）、
ひな形の実体（環境のファイル）は agent-control-plane のルートにあった。ルートのファイルは control-plane が自分のために埋めた
版で、共通の部分にも control-plane だけの形がある。新しいターゲットを作る人は、control-plane の文書を読んでその形を
手で戻していた（#310）。

依頼者の方針（#15）は、一つのディレクトリをそのまま写せばターゲットとして始められるようにし、target-template の
ADR や文書も、写したあとのターゲットに持ち続けることだった。

## Decision

- target-template の区切りは、最上位の `target/` に置く。`target/` は写す元で、その下の並びは、写したターゲットの
  リポジトリのルートの並びと同じにする。
- target-template の文書（ADR を含む）は `target/template/docs/` に置く。写したターゲットでは `template/docs/` になり、
  ターゲット自身の文書の根（`docs/`）と分かれる。ターゲットは、ひな形を新しくするときに `template/` を差し替える。
- いずれ別のリポジトリ（agent-target-template）に切り出すときは、`target/` の下がそのリポジトリのルートになる。
- agent-control-plane のルートの環境のファイルは、control-plane が埋めた版として残る。

## Alternatives

- **`template/` を写す元にし、文書を `template/target/docs/` に置く**：写した先で文書が `target/docs/` になる。
  ひな形から来た文書は、来た元の名前（`template/`）の下にあるほうが、ターゲット自身の文書と分けて読め、区切りの名前
  （target-template）とも合う。
- **ひな形の実体をルートに置いたまま、ひな形と違う所を並べる、ルートのファイルに印を付ける、写して直す作業を道具に
  する**：どれも、ルートと写す元の違いを手当てするだけで、違いそのものは残る（#310 で選ばなかった）。
- **今すぐ別のリポジトリに切り出す**：platform への相対リンク、検査、CI を、リポジトリをまたいで組み直すことになる。
  同居のまま写す元の形を先に作り、切り出しはそのディレクトリを持ち出すだけにする。

## Consequences

- target-template の文書へのリンクは、どの区切りからも `target/template/docs/` を指す。
- 写したターゲットに持ち続ける文書から platform と control-plane への相対リンクは、写した先では切れる。その扱いは、
  この ADR では決めない。
- ルートの環境のファイルと、写す元に置く環境のファイルの共通の部分は、ずれうる。
