---
name: github
description: GitHub で PR を作る・更新するとき、push したとき、PR のレビューに対応するとき、PR の CI の失敗を調べるときに使う。
---

# GitHub で作業する

PR の本文の作成と更新は `prctl` で行う。本文には、どの head まで説明したかの印が隠して入り、hook が push の
直後とターンの終わりに、本文が古いままになっていないかを確かめる。

```bash
prctl status                              # このブランチの PR と、本文が今の head を説明しているか
prctl since                               # 前に本文を書いてから push したコミット（コメントの材料）
prctl create "<タイトル>" body.md            # push 済みのブランチの PR を作る
prctl update [--title "<タイトル>"] body.md comment.md   # 本文（とタイトル）を書き換え、変更点のコメントを付ける
```

## 本文とコメント

本文とコメントに何を書くかは、conventions の[書き方：PR](../../../conventions/docs/reference/writing.md#pull-requests)に
ある。要約すると、本文はこの PR が今何か（何をなぜ変えるか、どう確かめたか）を、コメントは前回の push からの変更と
その理由を書き、本文は追記せず書き換える。

PR を作るときは、タイトルと本文を書くだけでよい（`prctl create`）。作ったあとに push するたびに、タイトルと本文の
書き換えと、コメントの2つを `prctl update` で行う（範囲が変わったらタイトルも `--title` で直す）。コメントの理由は
書いた本人しか知らないので、`prctl since` の出す事実に理由を足して書く。
- 確かめていないことは、確かめていないと書く。
- `Closes #<番号>` など、リポジトリの決まりは本文に入れる。

## コミットと履歴

- コミットメッセージは、それだけで変更の理由が分かるように書く。マージ方法によっては、そのまま既定のブランチに残る。
- 履歴を整理して force push したら、本文を書き換え、何をなぜ整理したかをコメントに書く。

## レビューと CI

```bash
gh pr view <番号> --comments                                    # 会話のコメント
gh api 'repos/{owner}/{repo}/pulls/<番号>/comments'              # コードへのコメント
gh api 'repos/{owner}/{repo}/pulls/<番号>/comments/<ID>/replies' -f body=...   # それに返信する
gh pr checks <番号>                                              # CI の結果
gh run view <run ID> --log-failed                               # 失敗したジョブのログ
```

- 指摘には、そのレビューコメントへの返信として答える（直したならどのコミットで直したか、直さないなら理由）。直したら上の
  「本文とコメント」に沿って PR を更新する。
- CI が失敗したら、ログで原因を確かめてから直す。手元で同じ検査を実行できるなら、push の前に通す。

## 注意

- 引数なしの `gh pr view` は upstream から PR を探すので、別のブランチの PR を返すことがある。番号を指定するか
  `prctl status` を使う。
- 本文を `gh pr edit` や `gh pr create` で直接書くと、印が消えて古い扱いになる。`prctl` を使う。
