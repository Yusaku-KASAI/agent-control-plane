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
prctl update [--title "<タイトル>"] body.md comment.md   # 本文（とタイトル）を書き換え、変更点のコメントを付ける
prctl unreviewed                          # まだレビューしていない範囲（最後にレビューしたコミットから今の head まで）
prctl review <コミット> findings.md          # そのコミットまでのレビューの結果を、PR のレビューとして残す
prctl review --minor <コミット> note.md      # そのコミットまでを、細部の指摘への直しとして、レビューせずに残す
```

## 本文とコメント

本文とコメントに何を書くかは、conventions の[書き方：PR](../../../conventions/docs/reference/writing.md#pull-requests)に
ある。要約すると、本文はこの PR が今何か（何をなぜ変えるか、どう確かめたか）を、コメントは前回の push からの変更と
その理由を書き、本文は追記せず書き換える。

PR を作るときは、タイトルと本文を書くだけでよい（`prctl create`）。作ったあとに push で中身が変わったら、本文の
書き換えとコメントの2つを `prctl update` で行い、範囲が変わったときはタイトルも `--title` で直す。コメントの理由は
書いた本人しか知らないので、`prctl since` の出す事実に理由を足して書く。
- 確かめていないこと、`Closes #<番号>` も本文に書く（[書き方：PR](../../../conventions/docs/reference/writing.md#pull-requests)）。

## 差分のレビュー

自分の変更は、コミットしてから、PR を作る前と push のたびに、サブエージェントにレビューさせる（review スキル）。
`prctl review` には、サブエージェントが見た最後のコミットと、その指摘を渡す。そのあとに直したコミットは、次の
`prctl unreviewed` の範囲になる（[差分のレビュー](../../../conventions/docs/reference/reviews.md#diff)）。

- PR を作る前のレビューは、`prctl create` のあとに残す。指摘への答えは、`findings.md` の指摘の後ろに書く。
- 指摘への直しは、すべての指摘に答えを決めてから、まとめて push する。その範囲が細部の指摘への、意味の
  変わらない直しだけなら、レビューさせずに `prctl review --minor` で残す。`note.md` には、どのレビューの細部の
  指摘への直しかを書く。PR の最初のレビューは省けない（[差分のレビュー](../../../conventions/docs/reference/reviews.md#diff)）。
- ほかの push では、`prctl unreviewed` の範囲をレビューさせて残す。指摘への答えは、直した push の `prctl update` の
  コメントに書く（直しがなければ PR のコメント）。

## ほかの人の PR と既定のブランチを読む

ほかの人の PR や、既定のブランチの今の姿は、`prctl worktree` が作る一時的な worktree で、detached のまま読む。
作業領域の HEAD を動かすと、作者に向けた hook（本文、レビュー、CI）が自分のターンで働き、残ったときはセッションの
片付けも妨げる。

```bash
prctl worktree pr <番号>                 # PR の head を取り出す。置き場所、範囲、差分の見方、次のコマンドを出す
prctl review --pr <番号> <見た head> -     # 標準入力の指摘を、PR 全体のレビューとして残す
prctl worktree remove pr <番号>          # 終えたら消す
prctl worktree default                  # 既定のブランチの head を取り出す（全体のレビュー）
prctl worktree remove default
```

- 依頼の内容と意図は `gh pr view <番号>` の本文から読む。閉じる Issue は本文の `Closes #<番号>`。
- 検査は、取り出した worktree の中で回す。
- レビューは COMMENT だけにする（`prctl review` は COMMENT だけを付ける）。承認も変更の要求もしない。マージを決めるのは人。

## コミットと履歴

- コミットメッセージは、それだけで変更の理由が分かるように書く（[書き方：コミット](../../../conventions/docs/reference/writing.md#commits)）。
- 履歴を書き換えて force push したら、`prctl update` で本文を書き換え、何をなぜ書き換えたかをコメントに書く
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
  - 今の head の CI が失敗していると、hook がターンの終わりに止める。実行中、人の操作待ち（`action_required`）、
    取り消し（`incomplete`）では止めないが、報告にはその状態を書く。
- CI が失敗したら、上のコマンドでログを見て原因を確かめてから直す。

## 注意

- 引数なしの `gh pr view` は upstream から PR を探すので、別のブランチの PR を返すことがある。番号を指定するか
  `prctl status` を使う。
- 本文を `gh pr edit` や `gh pr create` で直接書くと、印が消えて古い扱いになる。`prctl` を使う。
- 履歴を書き換えて、レビューしたコミットが今の履歴になくなると、ブランチ全体がまだレビューしていない範囲になる。
- 人やほかのエージェントが PR に付けたレビューも、本文があれば、そのコミットまでレビューした扱いになる。行への
  コメントだけのレビューや返信は数えない。
