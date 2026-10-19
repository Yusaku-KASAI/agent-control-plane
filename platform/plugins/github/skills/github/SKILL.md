---
name: github
description: GitHub で PR を作る・更新するとき、push したとき、PR のレビューに対応するとき、PR の CI の失敗を調べるときに使う。
---

# GitHub で作業する

PR の本文の作成と更新と、差分のレビューの記録は `prctl` で行う。本文には、どの head まで説明したかの印が隠して入り、
レビューは見た最後のコミットに付けた PR のレビューになる。hook が push の直後とターンの終わりに、本文が古いままか、
レビューが今の head に追いついていないかを確かめる。

```bash
prctl status                              # このブランチの PR と、本文とレビューが今の head に追いついているか
prctl since                               # 前に本文を書いてから push したコミット（コメントの材料）
prctl create "<タイトル>" body.md            # push 済みのブランチの PR を作る
prctl create --base <下のブランチ> "<タイトル>" body.md   # まだマージされていない PR に重ねて作る
prctl update [--title "<タイトル>"] body.md comment.md   # 本文（とタイトル）を書き換え、変更点のコメントを付ける
prctl unreviewed                          # まだレビューしていない範囲（最後にレビューしたコミットから今の head まで）
prctl review <コミット> review.json         # そのコミットまでのレビューの結果を、PR のレビューとして残す
prctl answer <コミット> answers.json        # そのコミットまでのレビューの指摘すべてへの答えを、PR のコメントにする
prctl review --minor <コミット> note.md      # そのコミットまでを、レビューしない直しとして残す
prctl mode [normal|thorough]              # レビューの回し方（通常か念入り）を見る、変える
prctl ready                               # 終えてよいときだけ、PR の下書きを外す
prctl draft                               # 追加の依頼で続けるとき、PR を下書きに戻す
```

本文、コメント、レビューの JSON などの作業用のファイルは、作業ツリーの外（`mktemp -d` で作った場所）に置き、
`prctl` には絶対パスで渡す。作業ツリーに置くと、コミットに紛れたり、control-plane 経由では push を断られたりする
（[ハーネスの契約の push](../../../../docs/reference/harness.md#push "在りか")）。
control-plane 経由では、そこ（`mktemp -d` の場所）はコンテナが作り直されると消える
（[ハーネスの契約のコンテナの寿命](../../../../docs/reference/harness.md#lifetime "在りか")）。本文を書き換えるときは、前に書いた
下書きを探さず、`gh pr view <番号> --json body --jq .body` で今の本文を取り直して直す（古い印は `prctl` が外す）。

## 本文とコメント

本文とコメントに何を書くかは、conventions の[書き方：PR](../../../conventions/docs/reference/writing.md#pull-requests)に
ある。要約すると、本文はこの PR が今何かを、決まった節（`## 何を変えるか`、`## 当てはめた決まり`、`## どう確かめたか`、
`## 確かめていないこと`）に書き、コメントは前回の push からの変更とその理由を書き、本文は追記せず書き換える。節が
足りないか空なら、`prctl create` と `prctl update` が断る。

PR を作るときは、タイトルと本文を書くだけでよい（`prctl create`）。作ったあとに push で中身が変わったら、本文の
書き換えとコメントの2つを `prctl update` で行い、範囲が変わったときはタイトルも `--title` で直す。コメントの理由は
書いた本人しか知らないので、`prctl since` の出す事実に理由を足して書く。
- まだマージされていない PR の中身を前提にする変更だけは、その PR に重ねる（`--base`。重ねてよいときと、本文に書く
  こと、下に push したときの取り込みは[変更](../../../conventions/docs/reference/changes.md#changes)）。`prctl` は、
  ブランチが下のブランチの上にあり、下に開いている PR があるときだけ作る。重ねたあと、下が先に進んで取り込んで
  いないときと、下の PR が閉じたとき（マージされたか、されずに閉じたか）は、`prctl status` と hook が知らせる。
- PR が Issue を閉じるときは、`Closes #<番号>` も本文に書く（[書き方：PR](../../../conventions/docs/reference/writing.md#pull-requests)）。

## 差分のレビュー

自分の変更は、コミットしてから、PR を作る前と push で head が進んだときに、サブエージェント（reviewer ロールがあればそれ）にレビューさせる
（review スキルの「レビューを頼んで答える」）。回し方は、既定は通常で、念入りに回す PR には、`prctl create` の
すぐあとに `prctl mode thorough` でラベル `念入りのレビュー` を付ける（[回し方](../../../conventions/docs/reference/reviews.md#ways)）。
ラベルがなければ通常として扱われる。今の回し方は `prctl status` の `reviewed:` の行で分かる。`prctl review` には、サブエージェントが見た最後のコミットと、その結果の JSON（下）を渡す。そのあとに
直したコミットは、次の `prctl unreviewed` の範囲になる（[差分のレビュー](../../../conventions/docs/reference/reviews.md#diff)）。

- PR を作る前のレビューは、PR 全体の差分のレビュー（`"scope": "pr"`、すべての観点）にし、`prctl create` のあとに
  残す。指摘への答えは、`review.json` の `answers` に書く。
- 指摘への直しは、すべての指摘に答えを決めてから、まとめて push する。
- push で head が進んだら、通常なら PR 全体を、念入りなら `prctl unreviewed` の範囲（終える前の PR 全体のレビューの
  指摘への直しなら、PR 全体）をレビューさせて残す。通常の PR に
  `"scope": "since"` のレビューを残そうとすると、`prctl` が断る。ただし、範囲が細部の指摘への、意味の
  変わらない直しだけなら、レビューさせずに `prctl review --minor` で残す。`note.md` には、どのレビューの細部の
  指摘への直しかを書く。範囲の始まりがレビューでないとき（PR にレビューが一つもない、どれも今の head の履歴にない、
  `--minor` の記録のあと）は、`prctl` が断る。
- 念入りでは、観点ごとにサブエージェントを並べて頼み、返った結果を一つの JSON にまとめて残す。
- 通常の打ち切り（必須のない PR 全体のレビューが 3 回続いた）は `prctl` が数えない。打ち切った提案には、`直さない` で、理由を「打ち切り」で
  始めて答える（あとの全体のレビューが、それで探す）。
- PR を作ったあとのレビューの指摘への答えは、どちらの道でも、`prctl answer` でそのレビューへの答えのコメントにする。
  直した push の `prctl update` のコメントには、push で何をなぜ変えたかを書く。

- `prctl create` は PR を下書きで作る。終える前（依頼者に終えたと報告する前）に、PR 全体の差分のレビュー
  （`"scope": "pr"`）を回して[終える前](../../../conventions/docs/reference/reviews.md#ending)を満たし、`prctl ready` で
  下書きを外す。満たしていなければ `prctl ready` が断り、何が足りないかを言う（`prctl status` の `end:` でも見える）。
  下書きを外したあとに変更すると、ターンの終わりに hook が止める。PR 全体の差分のレビューが上限（10 回、
  [回し方](../../../conventions/docs/reference/reviews.md#ways)）に達したら、`prctl ready` が断るとき、または下書きを
  外したあとのターンの終わりに hook が、依頼者に返すよう言う。
  下書きを外したあとに追加の依頼で作業を続けるときは、`prctl draft` で下書きに戻す。

### 結果と答えの JSON

レビューの結果と答えは JSON で `prctl` に渡し、`prctl` が Markdown の表と、機械で読む隠した写しの両方を作る。形が
足りなければ `prctl` が断る。項目の中身は[指摘](../../../conventions/docs/reference/reviews.md#findings)と
[結果の形](../../../conventions/docs/reference/reviews.md#record)にある。

```json
{
  "scope": "since",
  "perspectives": ["正しさ", "文書とコメント"],
  "skipped": [{"perspective": "セキュリティ", "reason": "権限も秘密も変わらない"}],
  "note": "全体についての一言（任意）",
  "findings": [
    {"severity": "提案", "perspective": "正しさ", "location": "bin/prctl:120", "summary": "一行の要約",
     "basis": "状態を軸で洗い出す", "problem": "何が起きるか", "fix": "直し方の案", "verified": "動かして確かめた",
     "notes": "補足（任意）"}
  ],
  "answers": [{"finding": 1, "action": "直した", "commit": "abc1234"}]
}
```

- `scope` は `since`（前回のレビューから。念入りの PR だけ）か `pr`（PR 全体。`perspectives` はすべての観点）。`since` では、見なかった
  観点を `skipped` に理由と書く。指摘の `perspective` は見た観点のどれか。指摘がなければ `findings` は `[]`。
  head の履歴にまだレビューがなければ（PR の最初のレビュー、履歴を書き換えたあと）、`pr` でなければ断られる。
- `answers`（PR を作る前のレビューだけ）と、`prctl answer` に渡すリストは、指摘の番号ごとに `action` を
  `直した`（`commit`）、`直さない`（`reason`）、`Issue にした`（`issue`）、`答えた`（`reason`）のどれかにする。
  すべての指摘に答える。

## ほかの人の PR と既定のブランチを読む

ほかの人の PR や、既定のブランチの今の姿は、`prctl worktree` が作る一時的な worktree で、detached のまま読む。
作業領域の HEAD を動かすと、作者に向けた hook（本文、レビュー、CI）が自分のターンで働き、残ったときはセッションの
片付けも妨げる。

```bash
prctl worktree pr <番号>                 # PR の head を取り出す。置き場所、範囲、差分の見方、次のコマンドを出す
prctl review --pr <番号> <見た head> -     # 標準入力の結果の JSON（"scope": "pr"）を、PR のレビューとして残す
prctl worktree remove pr <番号>          # 終えたら消す
prctl worktree default                  # 既定のブランチの head を取り出す（全体のレビュー）
prctl worktree remove default
```

- 依頼の内容と意図は `gh pr view <番号>` の本文から読む。閉じる Issue は本文の `Closes #<番号>`。
- 検査は、取り出した worktree の中で回す。
- レビューは COMMENT だけにする（`prctl review` は COMMENT だけを付ける）。承認も変更の要求もしない。マージを決めるのは人。

## コミットと履歴

- コミットメッセージは、それだけで変更の理由が分かるように書く（[書き方：コミット](../../../conventions/docs/reference/writing.md#commits)）。
- push の決まりは[検査と履歴](../../../conventions/docs/reference/changes.md#checks)にある。「書き換える」は、push した
  コミットを rebase、amend、squash などで別のコミットに置き換えることで、`pull --rebase` で取り込むときに、まだ push
  していないコミットを付け替えることは含まない。
  - 書き換えていないのに `fetch first` か `non-fast-forward` で断られたら：`--force` で押し切らず、
    `git pull --rebase origin <ブランチ>`（まだ push していないコミットに merge があれば `--rebase=merges`。その
    merge で解いた衝突はもう一度解く。merge には衝突の解決のほかの変更を入れない）で取り込んでから push し直す。衝突が解けなければ、取り込みをやめて依頼者に返し、push していない
    コミットが残っていることを書く。
  - 書き換えて push するとき：直しはできるだけコミットを足して行う。書き換えてよいのは自分で作ったと確かめられるコミット
    だけ（作者が同じでも、確かめられないものはほかの書き手のもの。依頼者が範囲を示して頼んだときは、その範囲）。書き換える前に取り込み、`git push --force-with-lease --force-if-includes` で push する。
  - それが `stale info` か `remote ref updated since checkout` で断られたら：`pull --rebase` も、cherry-pick や
    `rebase --onto` で載せる形もとらず、書き換えた先頭を別のブランチに残して依頼者に返す。報告には、残したブランチの名前、
    断られたときのリモートの先頭、書き換えたコミットを書く。
- push したコミットを書き換えて force push したら、`prctl update` で本文を今の中身に合わせ、何をなぜ書き換えたかをコメントに書く
  （[検査と履歴](../../../conventions/docs/reference/changes.md#checks)）。

## レビューと CI

```bash
gh pr view <番号> --comments                                    # 会話のコメント
gh api 'repos/{owner}/{repo}/pulls/<番号>/comments'              # コードへのコメント
gh api 'repos/{owner}/{repo}/pulls/<番号>/comments/<ID>/replies' -f body=...   # それに返信する
prctl ci [--wait]                                               # 今の head の CI。--wait は終わるまで待つ
gh pr checks <番号>                                              # CI のジョブと、その run の URL
gh run view <run ID> --log-failed                               # 失敗したジョブのログ
```

- 指摘には、上のコマンドでそのレビューコメントに返信して答え、直したら PR を更新する（[差分のレビュー](../../../conventions/docs/reference/reviews.md#diff)）。
- push の前に手元で CI と同じ検査を通し、push のあとは CI を待たずに進む（[検査と履歴](../../../conventions/docs/reference/changes.md#checks)）。
  どのコマンドかは、ターゲットなら[ターゲットの契約の検査](../../../../../template/docs/reference/contract.md#checks "在りか")にある。
  - control-plane 経由で動くとき（[ハーネスの契約の動き方](../../../../docs/reference/harness.md "在りか")）は、push のあと
    `prctl ci --wait` をバックグラウンドで実行してから、ターンを終える。CI が終わると自分で始めるターンになるので、
    失敗していれば直し、結果を報告する。`timeout` や「no check run appeared」で終わったら、そのことを報告する。
  - `prctl ci --wait` は、push のたびにバックグラウンドで始める。一つのブランチに一つだけ動き、このクローンの
    `origin/<ブランチ>` が指す head（ふつうは最後に push した head）を追うので、ほかの待ちが動いていれば
    「already waiting」ですぐ終わる。前の待ちが終わった知らせがまだ届いていないこともあるので、動いていると思っても
    始める。同じ CI をフォアグラウンドでも待たず、終わりを Monitor や ScheduleWakeup で見張らない
    （[ハーネスの契約の依頼と回答](../../../../docs/reference/harness.md "在りか")）。
  - 今の head の CI が失敗していると、hook がターンの終わりに止める。実行中、人の操作待ち（`action_required`）、
    取り消し（`incomplete`）では止めないが、報告にはその状態を書く。
- CI が失敗したら、上のコマンドでログを見て原因を確かめてから直す。
- `check-protection` ジョブ（既定のブランチの守りの検査）が失敗したら、ログで分ける
  （[既定のブランチの守りの用意](../../../../../template/docs/guides/github.md "在りか")）。
  - PR が CODEOWNERS を変えていて、`NG` の行がその CODEOWNERS のことなら、PR の中で直す。
  - 読めない（`cannot read`、終了コード 2）こと自体は、守りの抜けではない。PR では直さず、そのことを添えて、
    回し直しを依頼者に頼む（developer の App は Actions を読むだけで、回し直せない）。回し直しても読めなければ、CI の
    トークンで読めないことでありうる（target-template の ADR 0016）ので、`cannot read` の行を添えて報告する。
    読めずに止まったログにも、それより前の `NG` の行があれば、それはほかの二つの分け方で扱う。
  - それ以外（PR が触れていないリポジトリの設定や、CODEOWNERS の持ち主）は PR の変更では直らない。PR では直さず
    （CODEOWNERS や検査を書き換えて通さない）、`NG` の行を添えて依頼者に報告する。

## 注意

- 引数なしの `gh pr view` は upstream から PR を探すので、別のブランチの PR を返すことがある。番号を指定するか
  `prctl status` を使う。
- 本文を `gh pr edit` や `gh pr create` で直接書くと、印が消えて古い扱いになる。`prctl` を使う。
- 履歴を書き換えて、レビューしたコミットが今の履歴になくなると、ブランチ全体がまだレビューしていない範囲になる。
- 人やほかのエージェントが PR に付けたレビューも、本文があれば、そのコミットまでレビューした扱いになる。行への
  コメントだけのレビューや返信は数えない。
