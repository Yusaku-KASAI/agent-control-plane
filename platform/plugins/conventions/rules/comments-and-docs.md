# 共通のルール

どのリポジトリでも、いつも守ること（conventions プラグインが、セッションとサブエージェントの始めに読み込ませる）。
リポジトリの CLAUDE.md と食い違うときは CLAUDE.md に従う。

ここは要約で、決まりは括弧の中の出典にある。出典は conventions と principles の文書（在りかはこの後に示す）。

## コメント、コミット、PR

- コードは自己説明的に書き、コメントはコードで表しきれない制約や理由だけにする。構成、方針、使い方は文書に書く
  （[書き方：コメント](../docs/reference/writing.md#comments)）。
- 変更の理由と経緯は、コミットメッセージと PR に書く（[書き方：コミット](../docs/reference/writing.md#commits)）。
- PR の本文は、ブランチの今の中身を説明し続ける（[書き方：PR](../docs/reference/writing.md#pull-requests)）。

## 変更

- 変更する前に今の姿と前提を確かめ、壊れうる場面を挙げ、PR の前に別の目にレビューさせ、確かめていないことは書く
  （[作業の進め方：変更の進め方](../docs/reference/workflow.md#how-to-change)）。

## 文書

- 同じ事実は一か所にだけ書き、ほかはリンクする（[原則：SSOT](../../principles/docs/reference/principles.md#ssot)）。
- 今の姿だけを書く。変更履歴、以前の姿、試して捨てた案、日付付きの観測は書かない。直すときは追記せず書き換える
  （[原則：今と経緯を分ける](../../principles/docs/reference/principles.md#present-state)）。
- 一つの言葉は一つの意味で使い、その言葉を定める区切りの用語集に従う
  （[原則：ユビキタス言語と用語集](../../principles/docs/reference/principles.md#glossary)）。
- 書く前に、区切り、種類、持ち主を決める（[文書の体系](../docs/architecture/documentation.md)、手順は write-docs スキル）。
- これらを守るためなら、文書とコメントは破壊的に変えてよい（[文書の体系：書き換え](../docs/architecture/documentation.md#rewrite)）。
