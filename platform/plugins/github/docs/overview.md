# github の全体像

## 構成

```
github/
  bin/prctl            PR の作成と本文の書き換え、レビューの記録、本文とレビューが head に追いついているかと CI の判定、hook の本体（判定は docs/reference/prctl.md）
  lib/review.jq        レビューの結果と答えの形の検査と、Markdown の表の作り方
  lib/markdown.jq      PR と Issue の本文の行の読み方（見出しとコードブロック）
  tests/prctl-test.sh  prctl の振る舞いの検査
  bin/check-protection 既定のブランチの守り（ルールセットと CODEOWNERS）が外れていないかの検査
  actions/check-protection/  check-protection を CI で回す GitHub Actions の複合アクション
  tests/check-protection-test.sh  check-protection の振る舞いの検査
  bin/check-issues     開いている Issue とロードマップが、課題の扱い方の整っている状態のうち判断の要らないものを満たすかの検査
  tests/check-issues-test.sh  check-issues の振る舞いの検査
  hooks/hooks.json     Bash のあと（PostToolUse）とターンの終わり（Stop）に prctl hook を呼ぶ
  skills/github/       PR の本文とコメントの書き方、差分のレビューの残し方、レビューと CI への対応、Issue のずれの挙げ方
```

## 構成要素

### `bin/prctl`

- **責務**：今のブランチの PR を作って本文を印付きで書き換え、差分のレビューと答えを PR に残し、本文とレビューが head に
  追いついているか、PR を終えてよいか、head の CI の状態を判定し、ほかの人の PR と既定のブランチを一時的な worktree に
  取り出す。
- **置くもの / 置かないもの**：判定の決まりは[参照](reference/prctl.md "在りか")に、サブコマンドの一覧は `prctl` の usage に置く。本文の書き方と、指摘の
  中身は置かず、スキルと conventions に任せる。判断は次の ADR にある。
  - 本文は今のブランチの姿に合わせて書き換え、push ごとの変更はコメントにする（[ADR 0002](../../../docs/adr/0002-pr-body-tracks-head.md)）。
    本文は決まった節を持つ（[ADR 0012](../../../docs/adr/0012-pr-body-sections.md)）。まだマージされていない PR に重ねて作れる
    （[ADR 0013](../../../docs/adr/0013-stacked-prs.md)）。
  - 差分のレビューは head に追いつかせ、PR のレビューとして残す（[ADR 0005](../../../docs/adr/0005-self-review-tracks-head.md)）。
    細部の指摘への直しだけの範囲は、レビューせずに記録する（[ADR 0009](../../../docs/adr/0009-minor-fixes-recorded-without-review.md)）。
    結果と答えは決まった形の JSON で受ける（[ADR 0011](../../../docs/adr/0011-review-records-from-json.md)）。回し方はラベルで
    見分ける（[ADR 0021](../../../docs/adr/0021-two-ways-of-reviewing-a-pr.md)）。終える前に、PR 全体の差分のレビューが終えてよい
    状態にしているかを確かめる（[ADR 0014](../../../docs/adr/0014-whole-pr-review-before-ending.md)）。
  - CI は待たずに進み、失敗したまま終えない（[ADR 0006](../../../docs/adr/0006-ci-failures-stop-the-turn.md)）。CI を待つのは
    一つのブランチに一つだけ（[ADR 0010](../../../docs/adr/0010-one-ci-wait-per-branch.md)）。各 check の最新の run だけを数える
    （[ADR 0017](../../../docs/adr/0017-latest-check-run-across-suites.md)）。
- **関係**：`git`、`gh`、`jq` を使う。振る舞いは `tests/prctl-test.sh`（偽の `gh` と使い捨てのリポジトリ）で確かめる。`gh` の認証はハーネスの契約で渡される（[ハーネスの契約](../../../docs/reference/harness.md)）。
- **当てはめた原則・パターン**：[単一の情報源](../../principles/docs/reference/principles.md#ssot)：どこまで説明したかは本文の印に、
  どこまでレビューしたかは PR のレビューの `commit_id` にだけ持つ。どちらも GitHub から読むので、セッションやコンテナを
  またいでも同じ答えになる。[事実と判定を分ける](../../principles/docs/reference/patterns.md#facts-and-judgement)：
  レビューの一覧は事実として一度だけ読み、`--minor` を使えるかと終えてよいかの判定はそれぞれ一か所にして、断る文と
  案内の文の両方をそこから作る。

### `lib/review.jq`

- **責務**：レビューの結果と答えの JSON の形の検査と、PR のレビューとコメントにする Markdown の作り方と、レビューの
  事実から PR を終えてよいかの判定（`final_errors`）。
- **置くもの / 置かないもの**：重さ、観点、答えの名前は、conventions の[指摘](../../conventions/docs/reference/reviews.md#findings)、
  [観点](../../conventions/docs/reference/reviews.md#perspectives)、[結果の形](../../conventions/docs/reference/reviews.md#record)の写しで、
  そこを変えたらここも直す（試験が照らす）。どの範囲をレビューするか、いつ答えるかの判定と、GitHub と git から事実を
  読むことは置かず、`bin/prctl` に任せる。
- **関係**：`bin/prctl` が `jq -L` で読み込む。
- **当てはめた原則・パターン**：[仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)：形が足りなければ
  記録させない（強制機能）。[単一の情報源](../../principles/docs/reference/principles.md#ssot)：表と隠した写しは、
  同じ JSON から作る。

### `lib/markdown.jq`

- **責務**：PR と Issue の本文の行を、コードブロックの中か、どの節の見出しかとともに読む（`body_lines`）。
- **置くもの / 置かないもの**：行の読み方だけ。どの節を求めるか、どの行を数えるかは、使う側（`bin/prctl` の本文の検査と
  依頼の Issue の取り出し、`bin/check-issues` のテーマの節）に置く。
- **関係**：`bin/prctl` と `bin/check-issues` が `jq -L` で読み込む。
- **当てはめた原則・パターン**：[単一の情報源](../../principles/docs/reference/principles.md#ssot)：PR の本文と Issue の本文で、
  見出しとコードブロックの見分け方がずれない。

### `hooks/hooks.json`

- **責務**：push のあと（Bash の PostToolUse）と、ターンの終わり（Stop）に、`prctl hook` を呼ぶ。
- **置くもの / 置かないもの**：呼び出しだけ。何を確かめて促すか、止めるかは[参照の hook](reference/prctl.md#hooks "在りか")にある。
- **関係**：`bin/prctl` を呼ぶ。
- **当てはめた原則・パターン**：[仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)：本文を書き換えること、差分を
  レビューさせること、CI の失敗を直すことをプロンプトに頼らず、hook で見つけて促す。[別の目で見る](../../principles/docs/reference/patterns.md#four-eyes)。

### `bin/check-issues`

- **責務**：リポジトリの開いている Issue とロードマップが、conventions の
  [課題の扱い方](../../conventions/docs/reference/issues.md#tidy)の「整っている状態」のうち、判断の要らないものを満たすかを確かめる。
  満たさない項目を `NG` の行で出し、終了コードは 0 が通過、1 がずれ、2 が GitHub を読めない。
- **置くもの / 置かないもの**：
  - 確かめるのは四つ。テーマの Issue（ラベル `theme`）とロードマップのほかの開いている Issue が、どこを変えるかのラベル
    （`theme`、`theme:*`、`canvas`、GitHub の既定のラベル（`isDefault`。conventions の課題の扱い方が使わないとする）のほかのラベル）と `theme:<名前>` を一つ以上ずつ持つこと。開いているテーマの Issue の本文が、
    [テーマ](../../conventions/docs/reference/issues.md#theme)の節（`## 狙い`、`## 目指す姿`、`## 進め方`）をこの順に一度ずつ
    持つこと（コードブロックの中の見出しは数えない。ほかの節はあってよい）。ピン留めした「ロードマップ」の
    `## 今の焦点` の節が、このリポジトリの開いている Issue だけを挙げていること（閉じた Issue、PR、このリポジトリにない
    番号はずれ。`<リポジトリ>#<番号>` のほかのリポジトリへの参照は数えない）。その節がなければ、それもずれとする。
    開いている Issue が、閉じた Issue に blocked by されていないこと。
  - ラベルの確かめは、ラベルの決まりや当たるテーマの Issue があるかを見ない（決まりがあるかは参照の文書の中身で、
    判断が要る）。そのため、それらがないリポジトリでは、整っている状態のずれでない `NG` も出る（conventions の
    [ADR 0033](../../conventions/docs/adr/0033-repositories-without-label-rules-and-others-comments.md)）。
  - 整っている状態のうち、上の四つのほかは確かめない（判断が要り、課題を整える手順に任せる。テーマの本文の中身が
    メンバーや overview と合っているかも含む）。
  - Issue を書き換えない。読むだけ。CI では回さない（Issue は PR と関わりなく変わる）。課題を整えるときとセッションを
    締めるときに回す（[ADR 0023](../../../docs/adr/0023-check-issues-on-demand.md)、[0024](../../../docs/adr/0024-focus-lists-issues-only.md)、[0033](../../../docs/adr/0033-check-issues-checks-theme-sections.md)）。
- **関係**：`gh` と `jq` を使い、本文の行は `lib/markdown.jq` で読む。振る舞いは `tests/check-issues-test.sh`（偽の `gh`）で確かめる。エージェントには github
  スキルの「Issue のずれを挙げる」が届ける。
- **当てはめた原則・パターン**：[仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)：ロードマップの今の焦点が古いまま残る
  ことを、人の気づきに頼らず見つける。

### `bin/check-protection`

- **責務**：リポジトリの既定のブランチに効くルールと CODEOWNERS が、[ターゲットの契約](../../../docs/reference/target.md#protection)の
  既定のブランチの守りを満たすかを確かめる（[target-template の ADR 0016](../../../../template/docs/adr/0016-protection-checked-in-ci.md)、
  [0017](../../../../template/docs/adr/0017-protection-check-follows-contract.md)）。満たさない項目を `NG` の行で出し、終了コードは
  0 が通過、1 が抜け、2 が GitHub を読めない。
- **置くもの / 置かないもの**：
  - 確かめる値は、その契約を API の側から読んだもので、契約を変えたらここも直す。platform は target-template を
    前提にしないので、target-template の用意の手順（`guides/protection.md`）ではなく契約に合わせる（conventions の
    [検査の扱い方](../../conventions/docs/reference/checks.md "在りか")）。
  - ルールは `rules/branches/<既定のブランチ>` から読む。リポジトリと organization のどのルールセットから来たものでも、
    効いているものだけを数える。効いているルールはどれも満たす必要があるので、求める値はどのルールから来てもよい。
  - 承認を人に限る仕組みは、コードオーナーの承認（CODEOWNERS が誤りなく、最初の規則が `*` で、持ち主のない規則が
    ない）か、すべてのファイル（`*`）に承認を 1 件以上求める team の、どちらかがあればよい。CODEOWNERS は、指定した
    ref（CI では `GITHUB_SHA`、PR ではマージしたときの版）のものを見る。CODEOWNERS がない（404。ref がないときも同じ）
    ことは、team の側で守っていれば抜けではない。CODEOWNERS の問題（誤り、規則がない、最初が `*` でない、持ち主の
    ない規則）は、コードオーナーの承認を求めているときだけ抜けにする。CODEOWNERS を読めないときは、在りかを飛ばさず 2 で止める。
  - バイパスは、ルールセットが `bypass_actors` を返したときだけ確かめる（管理者の役割 `RepositoryRole` 5 か
    `OrganizationAdmin` で、`pull_request` のときだけ）。返さないとき（ルールセットを書けない認証情報、CI の
    `GITHUB_TOKEN`）は、確かめていないと出して失敗にしない。
  - リポジトリの設定は変えない。読むだけ。
- **関係**：`gh` と `jq` を使う。振る舞いは `tests/check-protection-test.sh`（偽の `gh`）で確かめる。
- **当てはめた原則・パターン**：[仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)：守りが黙って外れることを、
  CI の必須の検査で見つける。

### CI の守りの検査（[`actions/check-protection/`](../actions/check-protection/action.yml "在りか")）

- **責務**：ターゲットの CI で、そのリポジトリについて、アクションと同じ版の `bin/check-protection` を回す。
- **置くもの / 置かないもの**：`gh` に渡すトークン（既定は `github.token`）と、`check-protection` の呼び出しだけ。
  リポジトリはチェックアウトしない（GitHub を読むだけ）。
- **関係**：ターゲットの `ci.yml` が `@main` で呼ぶ。ランナーの `gh` と `jq` で動く。
- **当てはめた原則・パターン**：[SSOT](../../principles/docs/reference/principles.md#ssot)：検査の具体を platform の一か所に置き、ターゲットは
  呼ぶ一行だけを書く。

### `skills/github/`

- **責務**：PR の本文とコメントの書き方、差分のレビューの残し方、レビューと CI への対応、Issue のずれの挙げ方の手順。
- **置くもの / 置かないもの**：エージェントが従う手順とコマンド。`prctl` と hook の仕組みの説明はこの文書と[参照](reference/prctl.md "在りか")に置く。
- **関係**：エージェントが PR を作る・更新するとき、ほかの人の PR を読んでレビューを残すとき、課題を整えるときと
  セッションを締めるときに Issue のずれを挙げるときに使う。
