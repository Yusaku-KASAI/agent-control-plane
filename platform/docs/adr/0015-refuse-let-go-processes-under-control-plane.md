---
status: accepted
date: 2026-10-16
amends: [8, 10]
---

# 0015. control-plane 経由では、処理を手放す Bash と、終わりを見張るツールを hook で断る

## Context

ADR 0010 で、バックグラウンドの処理の終わりは知らせで届くので、Monitor や ScheduleWakeup で見張らないことを、
ハーネスの契約に決まりとして書いた。仕組みでは守らなかった。

このセッションで、`prctl ci --wait` を `&` で始めた（#105 のセッションに続いて 2 回目）。`&`、`nohup`、`disown`、
`setsid` で手放した処理は、終わりの知らせが届かないだけでなく、control-plane のバックグラウンドの処理にも数えられない。
依頼者は、決まりだけでなく仕組みで止めることを求めた（Issue #110）。

## Decision

- harness プラグインに PreToolUse の hook（`scripts/detach`）を置く。control-plane 経由のときは、次を断り、
  `run_in_background` を使うよう案内する。
  - Bash のコマンドのうち、処理を手放すもの。ヒアドキュメントの中身、引用符の中、コメントを除いた残りに、演算子の
    `&`（`&&`、`|&`、リダイレクトの `&` は除く）か、`nohup`、`disown`、`setsid` があるもの。`&` のあとに同じコマンド
    の中で `wait` するもの（コマンドの中で終わる）は通す。
  - Monitor と ScheduleWakeup。
- devcontainer で直接動くときは、何もしない。
- ハーネスの契約に、`run_in_background` を使うことと、hook が断ることを書く。

## Alternatives

- **決まりのままにする**：二度破られた（仕組みで守る）。
- **シェルの文法をきちんと解く**：hook に解析器を持ち込むことになる。字面で見て、まれに通すべきものを断る方を
  とり、断る文で道を案内する。
- **`&` を見つけたら、`run_in_background` に書き換えて通す**：コマンドの中の一部だけを手放す形は、書き換えると
  意味が変わる。断って、エージェントに書き直させる。
- **Monitor を残す**：開発用サーバーの出力を見るなどの使い道はあるが、control-plane 経由ではターンを増やし、回答を
  増やす。要れば、終わる処理として `run_in_background` で動かす。

## Consequences

- 手放す形で始めた処理は、control-plane 経由では動かない。終わりの知らせが届き、control-plane も数える形だけになる。
- 字面で見るので、引用符をまたぐ書き方やシェルの関数の中など、まれに通すべきものを断ることがある。断る文の案内で
  書き直す。逆に、`eval` や別のスクリプトの中で手放すものは見つけられない。
- Monitor と ScheduleWakeup は、control-plane 経由では使えなくなる。
