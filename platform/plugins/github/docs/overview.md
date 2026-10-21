# github の全体像

PR の本文は、追記ではなく今のブランチの姿に合わせて書き換え、push ごとの変更はコメントにする
（[ADR 0002](../../../docs/adr/0002-pr-body-tracks-head.md)）。本文は決まった節を持つ
（[ADR 0012](../../../docs/adr/0012-pr-body-sections.md)）。まだマージされていない PR に重ねて作れる
（[ADR 0013](../../../docs/adr/0013-stacked-prs.md)）。終える前に、PR 全体の差分のレビューが PR を終えてよい状態にしているかを
確かめる（[ADR 0014](../../../docs/adr/0014-whole-pr-review-before-ending.md)）。差分のレビューは、PR の head に追いつかせ、結果を PR の
レビューとして残す（[ADR 0005](../../../docs/adr/0005-self-review-tracks-head.md)）。細部の指摘への直しだけの範囲は、
レビューせずにそう記録して追いつかせる（[ADR 0009](../../../docs/adr/0009-minor-fixes-recorded-without-review.md)）。差分のレビューの回し方（通常と
念入り）は PR のラベルで見分け、回し方ごとに残してよいレビューと案内を変える
（[ADR 0021](../../../docs/adr/0021-two-ways-of-reviewing-a-pr.md)）。
CI は待たずに進み、失敗したまま終えない（[ADR 0006](../../../docs/adr/0006-ci-failures-stop-the-turn.md)）。CI を待つのは、
一つのブランチに一つだけにする（[ADR 0010](../../../docs/adr/0010-one-ci-wait-per-branch.md)）。レビューの結果と答えは決まった
形の JSON で受け、表と機械で読む写しを作る（[ADR 0011](../../../docs/adr/0011-review-records-from-json.md)）。このプラグインは、
それを `prctl` と hook で守らせる。あわせて、ターゲットの既定のブランチの守りが外れていないことを確かめる
`check-protection` と、それを CI で回す複合アクションを配る（[target-template の ADR 0016](../../../../template/docs/adr/0016-protection-checked-in-ci.md)）。

## 構成

```
github/
  bin/prctl            PR の作成と本文の書き換え、レビューの記録、本文とレビューが head に追いついているかと CI の判定、hook の本体
  lib/review.jq        レビューの結果と答えの形の検査と、Markdown の表の作り方
  tests/prctl-test.sh  prctl の振る舞いの検査（make check-platform が回す）
  bin/check-protection 既定のブランチの守り（ルールセットと CODEOWNERS）が外れていないかの検査
  actions/check-protection/  check-protection を CI で回す GitHub Actions の複合アクション
  tests/check-protection-test.sh  check-protection の振る舞いの検査（make check-platform が回す）
  hooks/hooks.json     Bash のあと（PostToolUse）とターンの終わり（Stop）に prctl hook を呼ぶ
  skills/github/       PR の本文とコメントの書き方、差分のレビューの残し方、レビューと CI への対応
```

- `prctl` は本文に、どの head コミットまで説明したかの印（`<!-- prctl: head=<コミット> -->`）を隠して入れる。レビューは、
  見た最後のコミットに付けた PR のレビュー（`commit_id`）として残す。どちらも GitHub から読むので、セッションや
  コンテナをまたいでも同じ答えになる。
- hook は push のあとに、本文が古いことや、レビューが追いついていないことを見つけて促し、ターンの終わりには CI の
  失敗も見つける。スキルはどう書くかを教える。

## 構成要素

### `bin/prctl`

- **責務**：今のブランチの開いている PR を作り（`--base` でほかの PR に重ねても）、本文を印付きで書き換え、
  レビューの結果を PR のレビューとして残し
  （`--pr` でほかの人の PR にも、`--minor` でレビューしない直しの範囲も）、ほかの人の PR と既定のブランチを
  一時的な worktree に取り出し、本文とレビューが head に追いついているかと、head の CI（check run）の状態を判定する。
  レビューへの答えを PR のコメントにする（`answer`）。
- **置くもの / 置かないもの**：PR はブランチ名で探す（引数なしの `gh pr view` は upstream をたどり、別のブランチの PR を
  返すことがある）。本文は、決まった節（conventions の
  [書き方：PR](../../conventions/docs/reference/writing.md#pull-requests)）がこの順に一度ずつあり、どれも空でないことを
  確かめてから作る・書き換える（節の名前はその写しで、コードブロックの中の行は見出しに数えない）。書き換えは
  push 済みの HEAD に対してだけ行い、前の印から push があったときは、その変更を説明する
  コメントを必ず付ける。まだレビューしていない範囲は、本文のある PR のレビュー（誰が付けたものでもよい）が付いたコミットのうち、
  今の head の履歴にある最後のものから、今の head まで。ないときは、ブランチが PR の向き先のブランチから分かれた所から。
  レビューしない直しの記録も PR のレビューなので、同じに数える。その記録は、範囲の始まりがレビューでないとき
  （PR にレビューが一つもない、どれも今の head の履歴にない、レビューしない直しの記録のあと）は断る。レビューの
  一覧は一度だけ読んで事実（コミットと、レビューか記録か）にし、使えるかの判定は一か所に置いて、断るときと
  案内するときに使う。
  レビューを読めないときは、コマンドは失敗し、hook は何も言わない。CI を読めないときは、CI のことだけを言わない。
  CI の状態は、失敗、実行中、人の操作待ち、取り消し、通った、なしに分け、各 check の最新の run だけを数える。同じ head に
  check suite が二つ以上あっても、同じ app の同じ名前の check run は一番新しいものだけを数える
  （[ADR 0017](../../../docs/adr/0017-latest-check-run-across-suites.md)）。
  CI を待つ（`ci --wait`）のは一つのブランチに一つだけで、コンテナの `TMPDIR`（なければ `/tmp`）の中のファイルの `flock` で守る
  （`flock` がないか、取り合い以外で失敗したら守らない）。待つ head は、このクローンの参照（`origin/<ブランチ>`）が
  指す head。ふつうはこのクローンから最後に push した head で、fetch で取り込んだほかからの push も、参照が指せば
  待つ。待つ間にその参照が動けば（push で進めても戻しても、fetch でも）、終わる直前でも、新しい head を初めから
  待ち直す。重ねた PR（向き先が既定のブランチ `origin/HEAD` でない）は、向き先のブランチの PR がマージされたか、
  マージされずに閉じたか、`origin/<向き先>` が head の履歴にないかを見て知らせる。`origin/HEAD` が手元にない、
  GitHub を読めない、head が手元にないときは何も言わない。
  終えてよいかは、head の履歴にあるレビューの事実（コミット、レビューしない直しの記録か、写しの範囲の種類と
  指摘の重さ、写しと答えのコメントから分かる答えた番号）を一度だけ読み、`lib/review.jq` の一か所で判定する。
  最後の PR 全体の差分のレビューのあとに、それより後のコミットの記録（レビューしない直しの記録は除く）と、まだ
  記録のない head がない（まだ記録のない push は、先に記録するよう言う）、
  必須の指摘がない、すべてに答えた（答えはレビューの id で突き合わせる）、のどれかが欠ければ終えられない。
  回し方は、PR のラベル `念入りのレビュー` があれば念入り、なければ通常と読み（`mode` が付け外しする）、通常の PR には
  前回からの差分のレビュー（`"scope": "since"`）を残させず、hook と `unreviewed` の案内も PR 全体のレビューを促す。
  通常の打ち切りは数えず、終える条件は回し方によらない。終えられないまま PR 全体の差分のレビューが上限
  （`PRCTL_FINAL_CAP`、既定は 10）に達したら、依頼者に返すよう言い、最後に数えた
  PR 全体の差分のレビューのコミットを作業ツリーの `.git` に覚えて、そのあとから数え直す（そのコミットが head の履歴に
  なければ、すべてを数える）。数え直す位置を覚えるのは、ターンの終わりに返すと言ったとき
  （下書きのあいだも、止めずに覚える）と、下書きでない PR で終えてよいと確かめたとき（ほかにターンを止めるものが
  ないとき）と、`ready` が下書きを外したとき。PR は下書きで作り
  （`create`）、`ready` は終えてよいときだけ下書きを外し、`draft` は下書きに戻す（どちらも GraphQL）。下書きの
  あいだは、hook は止めず、head を済んだとも覚えない。
  レビューの結果と答えは JSON で受け、`lib/review.jq` で形を検査して表を作り、JSON の写しを base64 にして
  本文の最後の行の隠した印（`<!-- prctl-review: … -->`、`<!-- prctl-answers: … -->`）に入れる。レビューの写しは、
  範囲の種類、見た観点、指摘ごとの重さと観点と要約だけにする（本文は GitHub の上限の 65536 文字まで。超えれば
  断る）。印は本文の最後の行からだけ読む（指摘の文が印を引いていてもよい）。head の履歴にまだレビューがなければ、
  PR 全体の差分のレビューを求める。答えは、`prctl` が残したレビューに対してだけ受け、その指摘すべてに、一つずつ
  答えることを求める。本文の書き方と、指摘の中身は置かず、スキルと conventions に
  任せる。
- **関係**：`git`、`gh`、`jq` を使う。振る舞いは `tests/prctl-test.sh`（偽の `gh` と使い捨てのリポジトリ）で確かめる。`gh` の認証はハーネスの契約で渡される（[ハーネスの契約](../../../docs/reference/harness.md)）。
- **当てはめた原則・パターン**：[単一の情報源](../../principles/docs/reference/principles.md#ssot)：どこまで説明したかは本文の印に、
  どこまでレビューしたかは PR のレビューの `commit_id` にだけ持つ。[事実と判定を分ける](../../principles/docs/reference/patterns.md#facts-and-judgement)：
  レビューの一覧は事実として一度だけ読み、`--minor` を使えるかの判定は一か所にして、断る文と案内の文の両方を
  そこから作る。

### `lib/review.jq`

- **責務**：レビューの結果と答えの JSON の形の検査と、PR のレビューとコメントにする Markdown の作り方。
- **置くもの / 置かないもの**：重さと観点の名前は、conventions の[指摘](../../conventions/docs/reference/reviews.md#findings)と
  [観点](../../conventions/docs/reference/reviews.md#perspectives)の写しで、そこを変えたらここも直す。どの範囲をレビュー
  するか、いつ答えるかの判定は置かず、`bin/prctl` に任せる。
- **関係**：`bin/prctl` が `jq -L` で読み込む。
- **当てはめた原則・パターン**：[仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)：形が足りなければ
  記録させない（強制機能）。[単一の情報源](../../principles/docs/reference/principles.md#ssot)：表と隠した写しは、
  同じ JSON から作る。

### `hooks/hooks.json`

- **責務**：push のあとと、ターンの終わりに、本文が古いこと、レビューが head に追いついていないこと、CI が失敗したことを
  見つける。
- **置くもの / 置かないもの**：
  - Bash のあと（`prctl hook post-tool-use`）：今の HEAD が push 済みで、PR の本文がそれを説明していないか、レビューが
    それに追いついていなければ、足りないものを促す文脈を足す。一つの push には一度だけ促す。
  - ターンの終わり（`prctl hook stop`）：この作業ツリーから push した head について、本文が説明していないか、レビューが
    追いついていないか、CI が失敗していれば、ターンを終わらせず、足りないものを済ませて依頼への回答全体をまとめ直す
    よう求める。CI の失敗のほかの状態では止めない。下書きでなければ、PR を終えてよいか（PR 全体の差分の
    レビューが済んでいるか）も確かめ、ほかの足りないものといっしょに言って止める（hook は一つのターンで一度しか
    止めないので、ほかを済ませたあとに確かめずに終わらないように）。まだ記録のない push は、先に記録するよう言うだけ
    で、上限に数えない。下書きの PR では確かめず、head を覚えない。本文とレビューが済み、終えてよく、CI が通った
    （またはワークフローがない）と確かめた head は、そのときのレビューの数といっしょに覚えて、同じ head とレビューの
    あいだは CI と答えのコメントを聞き直さない（PR とレビューの一覧は毎回読み、同じ head にレビューが足されれば
    確かめ直す）。
    hook に促されて続けたターンでは、もう止めない。
  - どちらも自分のエラー（PR がない、`gh` やネットワークが使えない）でツールの呼び出しやターンを失敗させない。覚えた
    head は作業ツリーの `.git` の中に置く。
- **関係**：`bin/prctl` を呼ぶ。
- **当てはめた原則・パターン**：[仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)：本文を書き換えること、差分を
  レビューさせること、CI の失敗を直すことをプロンプトに頼らず、hook で見つけて促す。[別の目で見る](../../principles/docs/reference/patterns.md#four-eyes)。

### `bin/check-protection`

- **責務**：リポジトリの既定のブランチに効くルールと CODEOWNERS が、[ターゲットの契約](../../../../template/docs/reference/contract.md#protection)の
  既定のブランチの守りを満たすかを確かめる。満たさない項目を
  `NG` の行で出し、終了コードは 0 が通過、1 が抜け、2 が GitHub を読めない。
- **置くもの / 置かないもの**：
  - 確かめる値は、その契約を API の側から読んだもので、契約を変えたらここも直す。platform は target-template を
    前提にしないので、target-template の用意の手順（`guides/github.md`）ではなく契約に合わせる（conventions の
    [検査にするものと置き場所](../../conventions/docs/architecture/documentation.md#checks "在りか")）。
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
- **関係**：ターゲットの `ci.yml` が `@main` で呼ぶ（target-template の [CI](../../../../template/docs/architecture/ci.md "在りか")）。
  ランナーの `gh` と `jq` で動く。
- **当てはめた原則・パターン**：[SSOT](../../principles/docs/reference/principles.md#ssot)：検査の具体を platform の一か所に置き、ターゲットは
  呼ぶ一行だけを書く。

### `skills/github/`

- **責務**：PR の本文とコメントの書き方、差分のレビューの残し方、レビューと CI への対応の手順。
- **置くもの / 置かないもの**：エージェントが従う手順とコマンド。`prctl` と hook の仕組みの説明はこの文書に置く。
- **関係**：roles の developer が PR を作る・更新するときに使う。
