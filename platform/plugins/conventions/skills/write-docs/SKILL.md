---
name: write-docs
description: 文書（README、CLAUDE.md、docs/ の下、ADR）を足したり書き換えたりするときに使う。どこに、どの種類で、どの形で書くかを決める手順。
---

# 文書を書く

文書を足したり書き換えたりするときの手順。分け方の全体は[文書の体系](../../docs/architecture/documentation.md "在りか")にある。

1. **区切りを決める**：誰が読む必要があるか。読む必要がある人すべてに届く区切りに置く。どの区切りにも要るなら
   共有の核に置き、特定の区切りの間で要るなら契約にする（[区切りと依存の向き](../../docs/architecture/documentation.md#contexts)）。
2. **種類を決める**：解説、手順、参照、判断のどれか。混ざるなら分ける（[文書の種類](../../docs/architecture/documentation.md#types)）。
   文書には今の姿だけを書き、理由は ADR に、経緯はコミットメッセージに、予定は Issue に置く
   （[書き方：コミット](../../docs/reference/writing.md#commits)、[課題の扱い方](../../docs/reference/issues.md)）。
3. **持ち主を探す**：その事実を持つ文書がすでにあるか。あればそこを書き換え、ほかからはリンクする。なければ、
   区切りと種類に合う文書に書くか、新しく作って `docs/README.md` に足す（[文書の根と器](../../docs/architecture/documentation.md#roots)、[形式：文書の根](../../docs/reference/formats.md#layout)、
   [SSOT](../../../principles/docs/reference/principles.md#ssot)）。
4. **形に合わせる**：アーキテクチャの文書の構成要素と ADR は[形式](../../docs/reference/formats.md)に沿う。言葉は[用語集](../../docs/reference/glossary.md)に従う。
5. **リンクを分ける**：在りかを示すだけのリンクには、タイトル「在りか」を付ける。リンク先の中身が変わったときに、
   リンクを含む文を見直す要があれば前提のリンク、なければ在りかのリンク（[形式](../../docs/reference/formats.md#links)）。全体（`overview.md`、
   索引）から子へのリンクは在りかのリンクにする。前提のリンクは、地図の前提の向きに沿わせる。
6. **地図を直す**：項目を足す、移す、消すとき、または項目が前提にするものが変わるときは、その親の地図（親の
   `overview.md`）を直す（[形式](../../docs/reference/formats.md#map)）。
7. **量を整える**：要らないもの、重複（ほかに持ち主があるもの）、経緯、置き場所違いのもの、古くなったものは、消すか
   持ち主へ移す。減らすのはこれらだけで、構成、言葉の定義、前提、条件や範囲は削らない。言い回しを短くするのは、
   意味、条件、範囲が変わらず、別の読み方ができないときだけにし、迷ったら長いままにする
   （[必要十分](../../../principles/docs/reference/principles.md#sufficiency)）。
   決まりを守るためなら、まとめ直す、移す、構成を変えるなど、破壊的に書き換えてよい
   （[書き換え](../../docs/architecture/documentation.md#rewrite)）。
8. **確かめる**：`make check-docs` を通す。
