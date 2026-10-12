# 共通のルール

どのリポジトリでも、いつも守ること（conventions プラグインが、セッションとサブエージェントの始めに読み込ませる）。
リポジトリの CLAUDE.md と食い違うときは CLAUDE.md に従う。

ここは要約で、決まりは括弧の中の出典にある。特定の作業（文書を書く、変更する）にだけ要る決まりは、その作業の
スキルと出典にある。

- コードは自己説明的に書き、コメントはコードで表しきれない制約や理由だけにする。構成、方針、使い方は文書に書く
  （[書き方：コメント](../docs/reference/writing.md#comments)）。
- 変更の理由と経緯はコミットメッセージと PR に書き、コードと文書には今の姿だけを書く
  （[書き方：コミット](../docs/reference/writing.md#commits)、[原則：今と経緯を分ける](../../principles/docs/reference/principles.md#present-state)）。
- 同じ事実は一か所にだけ書き、ほかはリンクする（[原則：SSOT](../../principles/docs/reference/principles.md#ssot)）。
- 一つの言葉は一つの意味で使い、その言葉を定める用語集に従う
  （[原則：ユビキタス言語と用語集](../../principles/docs/reference/principles.md#glossary)）。
- 確かめられなかったことは、確かめていないと書く（[変更の進め方](../docs/reference/changes.md#how-to-change)）。
- 変更するときは変更の進め方に、文書を書くときは write-docs スキルに従う（[変更の進め方](../docs/reference/changes.md#how-to-change)、
  [文書の体系](../docs/architecture/documentation.md)）。
