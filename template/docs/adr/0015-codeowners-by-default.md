---
status: accepted
date: 2026-10-18
amended_by: [16]
---

# 0015. 承認を人に限る CODEOWNERS を target-template に既定で置き、organization のターゲットでは消す

## Context

既定のブランチへのマージに数える承認は、人に限る（[app ADR 0027](../../../docs/adr/app/0027-require-human-approval-in-ruleset.md)）。
仕組みはリポジトリの持ち主で分かれ、個人のリポジトリでは `.github/CODEOWNERS` に人だけを書いてコードオーナーの承認を
求め、organization のリポジトリでは人だけの team をルールセットの必須のレビュアーにする。

target-template にもターゲットの契約にもこの用意がなく、新しいターゲットでは守りが抜けうる。守りが抜けても何も
失敗しないので、抜けたことに気づきにくい。CODEOWNERS は、行が誤っている、持ち主が書き込みの権限を失った、の
どちらでも黙って無視される。

## Decision

- target-template に `.github/CODEOWNERS`（`* @<持ち主のユーザー名>`）を「埋める」ファイルとして置く。ターゲットごとの
  部分は持ち主のユーザー名。
- organization のターゲットでは、このファイルを消し、人だけの team を必須のレビュアーにする。消すことも、
  ファイルの分類での「埋める」の埋め方として扱う。
- ルールセットの設定と確かめ方は target-template の手順（`guides/github.md`）に書き、ターゲットの契約では、守りを
  用意することだけを求める。

## Alternatives

- **既定では置かず、個人のターゲットで足す**：足し忘れても何も失敗せず、守りが抜けたままになる。置いてあれば、
  埋め忘れは手順の確かめ方（`codeowners/errors`）を回せば見つかり、消すことは organization のターゲットで意図して
  行う操作になる。
- **organization のターゲットでも CODEOWNERS に team を書く**：CODEOWNERS とルールセットの必須のレビュアーの
  二か所で同じことを決めることになり、片方だけ変えるとずれる。
- **分類に「任意」を足す**：「埋める」の中で消す場合を扱えば足り、分類を増やすほどの違いはない。

## Consequences

- 新しいターゲットは、target-template から始めれば CODEOWNERS を持つ。持ち主の名前を埋めて、ルールセットを
  手順どおりに設定する。
- 埋め忘れや誤りを見つけるのは、人が手順の確かめ方を回したときだけで、CI では落ちない。
- CODEOWNERS の共通の部分（コメントと、すべてのファイルを一つの持ち主にする形）を変えるときは、ほかのターゲットも
  追従する。
