---
status: accepted
date: 2026-10-24
amends: [9]
---

# 0048. 会話の履歴がある ID は、終えたターンが 0 でも `--resume` で起動する

## Context

ADR 0009 は、最初のターンを `--session-id`、以降を `--resume` で起動すると決め、記録の `turns`（終えたターンの数）で
切り替えてきた。Claude Code は、最初の依頼が届いた時点で会話の履歴（設定ディレクトリの `projects/<dir>/<ID>.jsonl`）を
書き、その ID を `--session-id` でもう一度始めようとすると `Session ID <ID> is already in use.` で断る。`turns` は
ターンを終えたときにしか増えないので、最初のターンの途中でプロセスが落ちる（コンテナが落ちる、control-plane が
再起動する）と、`turns` が 0 のまま履歴が残る。次の依頼は毎回 `--session-id` で断られる。Slack ではスレッドから ID が
決まるので、そのスレッドでは続けられなくなる。

## Decision

- 終えたターンが 1 以上なら、これまでどおり `--resume` で起動する。
- 終えたターンが 0 なら、エージェントの設定ディレクトリに、その ID の会話の履歴（`projects/*/<ID>.jsonl`）があるかを
  見る。あれば `--resume`、なければ `--session-id` で起動する。ID は UUID なので、Claude Code が作業ディレクトリから
  決める `<dir>` の名前の付け方は知らなくてよい。
- 判断は、Claude Code の起動の仕方を持つ runner（`internal/adapter/docker`）が `Start` で行う。

## Alternatives

- **履歴があるかだけで決め、`turns` を見ない**：control-plane が設定ディレクトリを一覧できないと（ユーザーの食い違いなど）、
  `filepath.Glob` は失敗を返さずに「ない」と答え、続いていたセッションまで `--session-id` で起動して断られる。今まで
  動いていたものを壊さないよう、`turns` が 1 以上のときは見ない。
- **`--session-id` が断られたら `--resume` で起動し直す**：Claude Code のエラーの文面に頼り、起動を二度試す処理が
  プロセスの扱いに入る。履歴を見れば、起動の前に決まる。
- **最初の依頼を送る前に、記録に「始めた」と書く**：プロセスが依頼を受け取る前に落ちると、履歴がないのに
  `--resume` で起動し、`No conversation found` で断られる。正は Claude Code が持つ履歴で、記録はその写しになる。

## Consequences

- 最初のターンの途中で落ちたセッションも、次の依頼で会話を引き継いで続けられる。
- runner は、Claude Code が会話の履歴を設定ディレクトリの `projects/*/<ID>.jsonl` に置くことを前提にする。片付け
  （`SessionFiles.Remove`）は、`<ID>` で始まる名前を消すだけで、この置き場所を知らない。Claude Code がこの置き場所を
  変えると、最初のターンの途中で落ちたセッションは、また続けられなくなる。
- 最初のターンの途中で落ちたセッションでは、`reported_usage` がなく `turns` が 0 なので、次のターンの使用量は、
  最初のターンの前と同じく 0 と比べて出す。Claude Code が再開で前のプロセスの合計を引き継ぐなら、落ちたターンの分も
  次のターンに含まれる。引き継ぐかは確かめていない。
