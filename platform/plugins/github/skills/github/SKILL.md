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
- push したら、`prctl unreviewed` の範囲をレビューさせて残す。指摘への答えは、直した push の `prctl update` の
  コメントに書く（直しがなければ PR のコメント）。

## コミットと履歴

- コミットメッセージは、それだけで変更の理由が分かるように書く（[書き方：コミット](../../../conventions/docs/reference/writing.md#commits)）。
- 履歴を書き換えて force push したら、`prctl update` で本文を書き換え、何をなぜ書き換えたかをコメントに書く
  （[検査と履歴](../../../conventions/docs/reference/changes.md#checks)）。

## レビューと CI

```bash
gh pr view <番号> --comments                                    # 会話のコメント
gh api 'repos/{owner}/{repo}/pulls/<番号>/comments'              # コードへのコメント
gh api 'repos/{owner}/{repo}/pulls/<番号>/comments/<ID>/replies' -f body=...   # それに返信する
gh pr checks <番号>                                              # CI の結果
gh run view <run ID> --log-failed                               # 失敗したジョブのログ
```

- 指摘には、上のコマンドでそのレビューコメントに返信して答え、直したら PR を更新する（[差分のレビュー](../../../conventions/docs/reference/reviews.md#diff)）。
- CI が失敗したら、上のコマンドでログを見て原因を確かめてから直す（[検査と履歴](../../../conventions/docs/reference/changes.md#checks)）。

## 注意

- 引数なしの `gh pr view` は upstream から PR を探すので、別のブランチの PR を返すことがある。番号を指定するか
  `prctl status` を使う。
- 本文を `gh pr edit` や `gh pr create` で直接書くと、印が消えて古い扱いになる。`prctl` を使う。
- 履歴を書き換えて、レビューしたコミットが今の履歴になくなると、ブランチ全体がまだレビューしていない範囲になる。
- 人やほかのエージェントが PR に付けたレビューも、本文があれば、そのコミットまでレビューした扱いになる。行への
  コメントだけのレビューや返信は数えない。
