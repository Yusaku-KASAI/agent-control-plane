# 共通のルール

どのリポジトリでも、いつも守ること（conventions プラグインが、セッションとサブエージェントの始めに読み込ませる）。
リポジトリの CLAUDE.md と食い違うときは CLAUDE.md に従う（[定義と届け方](../docs/architecture/documentation.md#delivery)）。

ここは要約と案内で、決まりは括弧の中のリンクの先にある。特定の作業（文書を書く、変更する）にだけ要る決まりは、定義の文書と、
その作業のスキルにある。リンクの直後に「（全体を読む）」があれば、その場面ではリンク先を全体まで読み、その中の
「（全体を読む）」もたどる（[読む範囲](../docs/architecture/documentation.md#reading-scope)）。

- 何かを決めるときは、まず一般に良いとされる確立した原則やパターンを探して沿い、その名前を挙げる
  （[原則：確立した考え方に拠る](../../principles/docs/reference/principles.md#established)）。今の形や頼まれた手段も、目的、
  より単純な形、より大きな枠組みから問い直す（[原則：前提を問い直す](../../principles/docs/reference/principles.md#double-loop)）。
- コードは自己説明的に書き、コメントはコードで表しきれない制約や理由だけにする。構成、方針、使い方は文書に書く
  （[書き方：コメント](../docs/reference/writing.md#comments)）。
- 変更の理由と経緯はコミットメッセージと PR に書き、コードと文書には今の姿だけを書く
  （[書き方：コミット](../docs/reference/writing.md#commits)、[原則：今と経緯を分ける](../../principles/docs/reference/principles.md#present-state)）。
- 同じ事実は一か所にだけ書き、ほかはリンクする（[原則：SSOT](../../principles/docs/reference/principles.md#ssot)）。決まりは
  conventions の `docs/` で定義し、ルール、ロール、スキルには、決まりを出典付きの要約か、在りかを示す案内として書く（[定義と届け方](../docs/architecture/documentation.md#delivery)）。
- 一つの言葉は一つの意味で使い、その言葉を定める用語集に従う
  （[原則：ユビキタス言語と用語集](../../principles/docs/reference/principles.md#glossary)）。
- 一つの変更は一つの目的にし、大きくなったら分ける（[変更とパターン](../docs/reference/changes.md#patterns)）。
- 確かめられなかったことは、PR や依頼への答えに、確かめていないと書く（[変更の進め方](../docs/reference/changes.md#how-to-change)）。
- 変更するときは変更の進め方に、文書を書くときは write-docs スキルに従う（[変更の進め方](../docs/reference/changes.md#how-to-change)）。
  決まりや文書を足す・移すとき、その置き場所を決めるとき、その差分をレビューするときは、
  [文書の体系](../docs/architecture/documentation.md "在りか")（全体を読む）。
