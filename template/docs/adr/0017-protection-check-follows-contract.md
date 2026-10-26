---
status: accepted
date: 2026-10-21
amends: [16]
---

# 0017. 既定のブランチの守りの検査は、用意の手順でなく契約に合わせる

## Context

ADR 0016 は、`check-protection`（platform の github プラグイン）が確かめる値を、target-template の用意の手順
（`guides/protection.md`）の設定の写しとした。platform は target-template を前提にしない（最上位の地図）ので、platform の
コマンドが target-template の手順に合わせるのは逆向きになる（conventions の ADR 0027）。守る中身は、ターゲットの契約の
「既定のブランチの守り」にもあるが、消せず force push できないことと、`check` の元を GitHub Actions に限ることは、
手順にだけあった。organization の管理者のバイパス（PR のとき）は、`check-protection` と ADR 0016 が通すのに、契約は
リポジトリの管理者だけと書いていた。

## Decision

- `check-protection` が確かめる値は、ターゲットの契約の「既定のブランチの守り」を API の側から読んだものにする。
  契約を変えたら、コマンドも直す。
- 契約に、既定のブランチは消せず force push できないことと、`check` を報告する元を GitHub Actions に限ること（ほかの
  元を許すと、コミットの状態を書ける誰でも成功を付けられる）を足し、バイパスに organization の管理者を含める。どれも
  今の手順と `check-protection` がすでに求めているか許しているもので、ターゲットに新しく求めることはない。
- 用意の手順（`guides/protection.md`）は、契約を満たす設定の仕方として残る。

## Alternatives

- **契約に守りの値を機械で読める形（表や JSON）で置き、`check-protection` がそれを読む**：写しのずれは仕組みで
  防げる。ずれはこの ADR の Context で一度見つかったが、検査が契約でなく手順に合わせていたためで、レビューで繰り返し
  見つかってはおらず、検査にするものの条件（実際に破られる）を満たさない。値はコマンドに直に持ち、どの決まりに
  合わせたかをコマンドに書いて、契約を変えたら直す。ずれが繰り返すようになったら、機械で読める形にする。
- **手順を platform に移す**：手順は、ターゲットのリポジトリの設定画面での作業で、ターゲットを用意する人が読む。
  契約を満たす方法の一つで、platform の能力ではない。

## Consequences

- 守りの中身を変えるときは、契約、用意の手順、`check-protection` の順に直す。
