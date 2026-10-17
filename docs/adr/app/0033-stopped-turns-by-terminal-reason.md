---
status: accepted
date: 2026-10-17
amends: [29]
---

# 0033. 止められたターンだけを中断とし、ほかの error_during_execution はエラーとして知らせる

## Context

ADR 0029 は、`result` の `subtype` が `error_during_execution` のターンを、すべて中断として扱った。Claude Code
（2.1.292）は、止められたときのほかに、ターンの途中の例外、ターンの準備の失敗、権限による停止でも、この `subtype`
を出す。そうしたエラーは、状態メッセージで「中断」と示され、依頼への返事は「依頼を中断しました。」になっていた。

Claude Code（2.1.292）の出力には、次のものがある。
- `result` の `terminal_reason`：ターンが終わった理由。止められたときは `aborted_streaming`（応答の受信中）か
  `aborted_tools`（ツールの実行中）。準備の失敗は `turn_setup_failed`。Claude Code 自身も、`error_during_execution`
  のうち `terminal_reason` が `aborted_*` ではないものを、エラーとして表示する。
- `subtype` が `error` で始まる `result` には `result` の文面がなく、`errors` にエラーの説明の文字列が並ぶ。
  `errors` には、Claude Code が自分の診断のために足す行（`[ede_diagnostic]` か `[session_crash]` で始まる）も
  入る。Claude Code 自身は、これらの行を表示しない。

## Decision

- `error_during_execution` で終わったターンのうち、次のどちらかに当たるものだけを中断とする。
  - control-plane が `interrupt` を送ってから、最初に届いた `result` のターン。
  - `terminal_reason` が `aborted_streaming` か `aborted_tools` のターン。
- それ以外の `error_during_execution` は、ADR 0029 のエラーで終わったターンとして知らせる。
- エラーで終わったターンの `result` の文面が空なら、`errors` から診断の行を除き、行ごとにつないだものを、
  エージェントの文面として引用する。

## Alternatives

- **`terminal_reason` だけで判定する**：`terminal_reason` は Claude Code の出力のスキーマで省略できる。control-plane
  が止めたことは自分で知っているので、`terminal_reason` がなくても、止めたターンは中断と分かる。
- **`interrupt` を送ったかだけで判定する**：control-plane のほかに止める道（Claude Code の中で止まったとき）があれば、
  それをエラーとして知らせてしまう。
- **これまでどおりすべて中断とする**：失敗が中断に見え、依頼者は直すべきことがあると気づかない。

## Consequences

- 例外や準備の失敗で終わったターンは、状態メッセージで失敗と示され、返事にはエラーの説明が引用される。
- `interrupt` を送ったことは、次に届いた `result` で使い切る。dispatcher は、ターンが動いているとみなしている間に
  だけ `interrupt` を送る。それでも `interrupt` がどのターンも止めず `result` が来なかったときは、次のターンが
  `terminal_reason` が `aborted_*` ではない `error_during_execution` で終わると、中断として知らされる。
- `terminal_reason` がない古い Claude Code では、control-plane が止めたときだけが中断になり、ほかの
  `error_during_execution` はエラーになる。
- 止めたときと準備の失敗のときの実際の出力は、Claude Code の実行ファイルの中身から読んだもので、実物で止めて
  確かめてはいない。
