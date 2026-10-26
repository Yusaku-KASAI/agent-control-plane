---
status: accepted
date: 2026-10-26
---

# 0045. 決まりを変える判断はその決まりを持つ区切りの ADR に置き、差分のレビューの決まりの判断を conventions からたどれるようにする

## Context

レビューの扱い方（conventions の `reference/reviews.md`）の差分のレビューの決まり（結果の形、頼み方、見た最後のコミットに
付けて残す、細部の直しはレビューしない、終える前、通常と念入り、打ち切り）は、platform の ADR で決めてきた。

- [0005](../../../../docs/adr/0005-self-review-tracks-head.md)：差分のレビューを、見た最後のコミットに付けた PR のレビューとして残す。
- [0011](../../../../docs/adr/0011-review-records-from-json.md)：結果の形（指摘の表と答えの名前）と、PR を作る前のレビューを PR 全体にすること。
- [0018](../../../../docs/adr/0018-reviewer-reviews-own-changes-too.md)：変更した人が頼むレビューの結果を返して GitHub に残さず、頼む文に手順の在りかを書くこと。
- [0009](../../../../docs/adr/0009-minor-fixes-recorded-without-review.md)：細部の指摘への直しだけの範囲は、レビューせずにそう記録する。
- [0014](../../../../docs/adr/0014-whole-pr-review-before-ending.md)：終える前に PR 全体の差分のレビューを求める。
- [0021](../../../../docs/adr/0021-two-ways-of-reviewing-a-pr.md)：通常と念入りの二つの回し方に分ける。
- [0025](../../../../docs/adr/0025-normal-review-cutoff-after-two.md)：通常の打ち切りを、必須のない PR 全体のレビューが 2 回続いたときにする。

conventions の文書からはこれらをたどれず、決まりを変えるときに、過去の理由と選ばなかった案を見落としうる。全体のレビュー
#283 の判断 6 で、依頼者はおすすめの (a) で進めると決めた（#284）。

## Decision

- 決まりを変える判断は、その決まりを持つ区切りの `adr/` に置く（形式の ADR の節）。道具の振る舞い（`prctl` が何を
  確かめるかなど）の判断は、その道具を持つ区切りの `adr/` に置き、決まりの判断を指す。
- 今の差分のレビューの決まりは、上の platform の ADR の判断をまとめたものとして、この ADR を conventions での起点にする。
  レビューの扱い方は、この ADR を判断の理由として指す。
- これから差分のレビューの決まり（回し方、打ち切り、終える前など）を変える判断は、conventions の ADR に置く。

## Alternatives

- **今のまま（判断 6 の (b)）**：決まりの持ち主と判断の置き場所が違う区切りに分かれたままになる。
- **platform の ADR を conventions に移す**：どれも、決まりの判断と `prctl` や hook の振る舞いの判断を一つに持つ。丸ごと
  移すと道具の判断が conventions に入り、この ADR の決定（道具の判断はその道具を持つ区切りに置く）に反する。決まりの
  部分だけを分けて移すと、これらの判断の理由と選ばなかった案を書き直す手間と、書き写した理由が元とずれるおそれがある。
  この ADR から指せばたどれ、判断 6 の目的（conventions からたどる）は満たせる。

## Consequences

- 決まりの理由をたどるとき、conventions の ADR から platform の ADR へ一段たどることがある。
