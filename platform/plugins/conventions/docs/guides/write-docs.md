# 文書を書く

文書（README、CLAUDE.md、`docs/` の下、ADR）を足したり書き換えたりするときの手順。共通の手順で、目的の手順の段の中で
使う（[目的の手順と共通の手順](../architecture/documentation.md#procedures)）。分け方の全体は
[文書の体系](../architecture/documentation.md "在りか")にある。

1. **区切りを決める**：誰が読む必要があるか。読む必要がある人すべてに届く区切りに置く。どの区切りにも要るなら
   共有の核に置き、特定の区切りの間で要るなら契約にする（[区切りと依存の向き](../architecture/documentation.md#contexts)）。
2. **種類を決める**：解説、手順、参照、判断のどれか。混ざるなら分ける（[文書の種類](../architecture/documentation.md#types)）。
   文書には今の姿だけを書き、理由は ADR に、経緯はコミットメッセージに、予定は Issue に置く
   （[書き方：コミット](../reference/writing.md#commits)、[課題の扱い方](../reference/issues.md)）。
3. **持ち主を探す**：その事実を持つ文書がすでにあるか。あればそこを書き換え、ほかからはリンクする。なければ、
   区切りと種類に合う文書に書くか、新しく作って `docs/README.md` に足す（[文書の根と器](../architecture/documentation.md#roots)、
   [形式：文書の根](../reference/formats.md#layout)、[SSOT](../../../principles/docs/reference/principles.md#ssot)）。
4. **形に合わせる**：アーキテクチャの文書の構成要素、手順（[目的の手順と共通の手順](../reference/formats.md#procedure)）、
   [役割ごとの指示](../reference/formats.md#role)、[スキル](../reference/formats.md#skill)、ADR は[形式](../reference/formats.md)に沿う。言葉は
   [用語集](../reference/glossary.md)に従う。
5. **触れ方とリンクを分ける**：ほかの文書の中身に触れるときは、要約か案内で書く
   （[要約と案内](../architecture/documentation.md#summaries-and-guides)）。同じ事実を細部まで持つ所は一つに絞り、ほかは要約か
   案内にする（[SSOT](../../../principles/docs/reference/principles.md#ssot)）。そのうえでリンクの種類を決める。リンク先の中身が
   変わったときに、リンクを含む文を見直す要があれば前提のリンク、なければ在りかのリンクにし、在りかのリンクには
   タイトル「在りか」を付ける（[形式](../reference/formats.md#links)）。全体（`overview.md`、索引）から子へのリンクは在りかの
   リンクにする。前提のリンクは、地図の前提の向きに沿わせる。
6. **地図を直す**：項目を足す、移す、消すとき、または項目が前提にするものが変わるときは、その親の地図（親の
   `overview.md`）を直す（[形式](../reference/formats.md#map)）。
7. **量を整える**：要らないもの、重複（ほかに持ち主があるもの）、経緯、置き場所違いのもの、古くなったものは、消すか
   持ち主へ移す。減らすのはこれらだけで、構成、言葉の定義、前提、条件や範囲は削らない。言い回しを短くするのは、
   意味、条件、範囲が変わらず、別の読み方ができないときだけにし、迷ったら長いままにする
   （[必要十分](../../../principles/docs/reference/principles.md#sufficiency)）。
   言い直しを外すときは、外す決まりの持ち主がほかにあるかを先に確かめる。なければ、その決まりを置くべき文書（3 段の
   持ち主）に定義として移してから外す。
   決まりを守るためなら、まとめ直す、移す、構成を変えるなど、破壊的に書き換えてよい
   （[書き換え](../architecture/documentation.md#rewrite)）。
8. **確かめる**：`check-docs`（conventions のコマンド）を通す。Markdown を変えたターンの終わりには hook も回す。
