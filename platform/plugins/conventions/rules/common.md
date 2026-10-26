# 共通のルール

どのリポジトリでも、いつも守ること。conventions プラグインが、セッションとサブエージェントの始めに読み込ませる。
リポジトリの CLAUDE.md と食い違うときは CLAUDE.md に従う（[定義と届け方](../docs/architecture/documentation.md#delivery)）。
ここは要約と案内で、定義は括弧の中のリンクの先にある。

## 始めに読む

- どの作業でも、始めに[文書の体系](../docs/architecture/documentation.md "在りか")を全体まで読む。このプロジェクト群の
  文書と決まりがどこにあり、この共通のルール、ロール、スキルが自分に何をどう届けているかを知るため
  （[起点](../docs/reference/delivery.md#starting-points)）。

## 振り分け

依頼を受けたら、どの目的の手順に当たるかを決める。作業の途中で目的が変わったら（相談から変更へなど）、その手順に
入り直す。ロールの指示に「受け持つ手順」の節があれば、その扱いに従う
（[手順と役割](../docs/architecture/documentation.md#procedures)）。

- 変更（PR）にする：その前に[変更を届ける](../docs/guides/deliver-change.md "在りか")と[変更の扱い方](../docs/reference/changes.md "在りか")を
  全体まで読み、変更を届けるの段の順に進める（[変更を届ける](../docs/guides/deliver-change.md)の 1 段）。
- ほかは、括弧の中のスキルを使い、その目的の手順に沿う：頼まれて差分や全体をレビューする（review、[レビューする](../docs/guides/review.md "在りか")）、
  セッションを締める（「締めて」など。close-session、[セッションを締める](../docs/guides/close-session.md "在りか")）、相談や壁打ち（consult、
  [相談に乗る](../docs/guides/consult.md "在りか")）、Issue やロードマップの整理（organize-issues、[課題を整える](../docs/guides/organize-issues.md "在りか")）。
- どれにも当たらない（事実を聞く質問など）：依頼に沿って進める。

## 決めるとき

- まず一般に良いとされる確立した原則やパターンを探して沿い、その名前を挙げる
  （[原則：確立した考え方に拠る](../../principles/docs/reference/principles.md#established)）。
- 今の形や頼まれた手段も、目的、より単純な形、より大きな枠組みから問い直す
  （[原則：前提を問い直す](../../principles/docs/reference/principles.md#double-loop)）。

## 書くとき

- コードは自己説明的に書き、コメントはコードで表しきれない制約と、その理由だけにする。構成、方針、使い方は文書に書く
  （[書き方：コメント](../docs/reference/writing.md#comments)）。
- 変更の理由と経緯はコミットメッセージと PR に書き、コードと文書には今の姿だけを書く
  （[今の姿と理由の置き場所](../docs/architecture/documentation.md#reasons)、[原則：今と経緯を分ける](../../principles/docs/reference/principles.md#present-state)）。
- 同じ事実は一か所にだけ書き、ほかはリンクする（[原則：SSOT](../../principles/docs/reference/principles.md#ssot)）。決まりは
  conventions の `docs/` で定義し、ルール、ロール、スキルには、決まりを出典付きの要約か、在りかを示す案内として書く（[定義と届け方](../docs/architecture/documentation.md#delivery)、[要約と案内](../docs/reference/delivery.md#summaries-and-guides)）。
- 一つの言葉は一つの意味で使い、その言葉を定める用語集に従う
  （[原則：ユビキタス言語と用語集](../../principles/docs/reference/principles.md#glossary)）。
- 文書を書くときは write-docs スキルに従う。

## 変更と報告

- 一つの変更は一つの目的にし、大きくなったら分ける（[変更とパターン](../docs/reference/changes.md#patterns)）。
- 確かめられなかったことは、PR や依頼への答えに、確かめていないと書く（[変更を届ける](../docs/guides/deliver-change.md)）。
