---
status: accepted
date: 2026-11-03
---

# 0020. 写したターゲットの target-template の文書は、外へのリンクを GitHub の main へのリンクにして持つ

## Context

ADR 0018 は、写したターゲットが target-template の文書（`template/docs/`）を持ち続けるとした。文書には platform と
control-plane へのリンクが 52 本あり（原則、ターゲットの契約、conventions の形式、control-plane の運用の手順など）、
agent-control-plane の中の相対パスなので、写した先では切れ、ターゲットの `check-docs` が落ちる。ADR 0018 はその扱いを
決めず、新しいターゲットを作る手順は `template/` を写さなかった。

## Decision

- 写す元では、リンクを相対パスのまま持つ。agent-control-plane の `check-docs` が、実在、節、地図の向きを確かめる。
- 写すときに（`template/bin/relink-template`。`copy-target` が呼ぶ）、`template/` の外を指すリンクを、
  agent-control-plane の GitHub の `main` の同じファイルへのリンクに書き換える。ターゲットは platform のアクションも `main`
  で使う（ADR 0014）。
- ADR の frontmatter の参照（`amends` など）のうち `template/` の外を指すものは、写した先では指すファイルがないので外す。
  本文の記述は残る。
- 写した先の最上位の地図（`docs/overview.md` のひな形）に、target-template の項目（`../template/`）を置き、入口
  （`README.md` と `CLAUDE.md` のひな形）から `template/docs/README.md` へリンクする。写す道具（`template/bin/`）は写さない。

## Alternatives

- **写す元のリンクも GitHub の URL にする**：agent-control-plane の `check-docs` が実在と地図の向きを確かめられなくなる。
- **写した先の配置（platform は隣の `agent-platform`）に合わせた相対パスにする**：写す元とターゲットでディレクトリの
  深さが違い、同じリンクで両方は指せない。ターゲットの中から外のファイルを指すので、ターゲットの検査でも確かめられない。
- **リンクを外し、名前だけで指す**：読み手が出典へたどれず、前提のリンクが表していた見直しの向きも消える。
- **写したコミットの版を指す**：その場の作業ツリーのコミットは GitHub にないことがあり、リンクが切れうる。

## Consequences

- 写した先の文書の外へのリンクは、`main` の今の版を指し、写したときの版とずれうる。
- target-template を別のリポジトリに切り出したら、書き換え先（`relink-template` の `REPOSITORY`）を変える。
- 写した先で、外へのリンクは外のリンクとして扱われ、ターゲットの `check-docs` は確かめない。
