---
status: accepted
date: 2026-11-04
---

# 0040. 既定のブランチへのマージの前に、PR のブランチが向き先の最新に合っていることを求める

## Context

ターゲットの既定のブランチは、ルールセットで PR 経由に限り、必須のチェック `check` と人の承認を求める
（[app ADR 0025](../../../docs/adr/app/0025-pr-review-as-the-last-line.md)、[app ADR 0027](../../../docs/adr/app/0027-require-human-approval-in-ruleset.md)）。
`check` が確かめるのは PR の head で、マージされるのは head と、その後に進んだ既定のブランチを合わせたものになる。
ルールセットは、ブランチが向き先の最新に合っていることを求めていなかった（`strict_required_status_checks_policy` が false）。

エージェントのセッションは並べて動き、PR も並べて開く。並べた PR が、それぞれでは `check` を通るのに、合わせると
落ちる変更を持つことがある。PR #361 と #364 は、どちらも `docs/adr/app/` に 0064 の ADR を足し、続けてマージされて
既定のブランチの check-docs が落ちた（PR #378 で直した）。PR #385 と #386 も、どちらも `platform/docs/adr/` に 0038 の
ADR を足して同じく落ちた（PR #388 で直した）。落ちているあいだは、既定のブランチの CI と、どのブランチの push の前の
`make check` も止まり、直す PR のほかは進められない（#380）。

ADR の番号は、重なる変更のうち一番起きやすいものだが、一つでしかない。ある PR が文書を移し、別の PR がその文書への
リンクを足すような組み合わせでも、同じように落ちる。

GitHub のマージキューは、合わせたものを検査してからマージする仕組みだが、organization のリポジトリ（公開のものか、
GitHub Enterprise Cloud のもの）でしか使えない。依頼者のターゲットは個人のリポジトリで使えない。

## Decision

- ターゲットの契約の既定のブランチの守りに、マージの前に PR のブランチが向き先の最新に合っていることを足す
  （ルールセットの Require status checks to pass の Require branches to be up to date before merging、API では
  `check` を求める `required_status_checks` のルールの `strict_required_status_checks_policy`）。`check` が確かめた
  ツリーが、そのままマージの結果のツリーになる。
- github プラグインの `check-protection` が、これを確かめる。
- 遅れた PR は、向き先を PR のブランチに取り込んで（merge）、`check` を回し直してからマージする。取り込むのは、PR の画面の
  Update branch か、PR のブランチに push できる人やエージェント。push で承認が取り消されるので（app ADR 0027）、
  取り込んだあとに承認し直す。

## Alternatives

- **マージキューを使う**：合わせたものを検査し、遅れた PR を取り込み直す手間もない。個人のリポジトリでは使えない。
  ターゲットが organization のリポジトリに移れば、この ADR を置き換えて選び直す。
- **ADR の番号の振り方を変える（日付、PR の番号、番号なしなど）**：ADR の重なりはなくなるが、ほかの重なる変更は残る。
  ADR を番号で参照する frontmatter（`amends` など）と、それを確かめる検査、すでにある ADR と参照も変えることになる。
- **既定のブランチの CI が落ちたら、直す PR を早く出す（今の形）**：重なりは毎回直せるが、直すまでのあいだ、どのブランチの
  push も止まる。同じことが続けて起きている。
- **ターゲットごとに選ばせる（契約に入れない）**：並べて動くエージェントのセッションは、どのターゲットでも同じなので、
  同じことがどこでも起きうる。守りを契約の外に置くと、`check-protection` で確かめられない。
- **検査で、PR の head ではなく向き先と合わせたものを確かめる（`pull_request` のイベントの既定の merge ref）**：GitHub
  Actions はすでに合わせたものを検査しているが、検査したときの向き先に合わせたもので、その後に向き先が進めば同じく
  ずれる。

## Consequences

- 並べた PR は、一つをマージするたびに、残りが遅れる。残りは取り込み、`check` を待ち、承認し直してからマージする。
  依頼者が一人で、並べる PR の数も少ないので、この手間を受け入れる。
- 取り込んで重なりが見つかれば、マージの前に、その PR の中で直す。
- 守りを変えるのはリポジトリの管理者で、エージェントの認証情報では変えられない。ターゲットの CI は `check-protection` を
  control-plane の既定のブランチから使うので、この判断が既定のブランチに入ると、守りを変えるまで、どのターゲットでも
  `check-protection` と `check` が落ちる。守りは、入れる前に変えておく（足した設定は、今の `check-protection` を落とさない）。
