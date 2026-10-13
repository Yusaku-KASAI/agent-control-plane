---
name: design
description: 状態によって振る舞いが変わる処理、消す・上書きする処理、データや設定の移行を設計するとき、またはその差分をレビューするときに使う。作る前に当てはめ、レビューでは抜けを探す観点として使う。
---

# 設計の観点

変更を設計するときと、差分をレビューするときに使う。どの変更にどのパターンを当てはめるかは
[変更とパターン](../../docs/reference/changes.md#patterns "案内")、レビューでの指摘の仕方は
[指摘](../../docs/reference/reviews.md#findings "案内")にある。

1. principles の原則「確立した考え方に拠る」に沿って決める。変更の種類を見て、[変更とパターン](../../docs/reference/changes.md#patterns)の表から当てはまるパターンを選ぶ。
2. 各パターンの形とよくある失敗を、principles のパターンの一覧（SessionStart で示された principles の `docs/README.md` と
   同じ `docs/` の `reference/patterns.md`）で確かめる。
3. 作るときはその形に沿って設計し、レビューでは守られていない点を、パターンの名前と具体的な組み合わせで指摘する。
