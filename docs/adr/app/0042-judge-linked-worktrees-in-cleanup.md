---
status: accepted
date: 2026-10-21
amends: [26]
---

# 0042. 片付けの判定で、作業領域の中のほかの worktree の作業ツリーの変更、途中の操作、detached HEAD も見る

## Context

片付けの判定（ADR 0026）は、作業ツリーの変更、途中の操作、detached HEAD を、作業領域の主の worktree でしか見て
いなかった。サブエージェントが `isolation: worktree` で作る worktree（作業領域の中の `.claude/worktrees/` の下。
ターゲットの `.gitignore` で ignore する）は、変更があると残る。そこにコミットしていない変更や、detached HEAD の
コミットを残したまま保持期間が過ぎると、作業領域ごと消えていた。worktree のブランチは `refs/heads` にあるので、
そこにコミットした作業は、今までもローカルのブランチとして見ていた。

git は worktree のパスを絶対パスで記録する。worktree を作るのはエージェントのコンテナの中なので、記録された
パスはコンテナから見たもの（`/workspace/<名前>/...`）で、control-plane から見た作業領域のパスとは違う。そのため
`git worktree list` のパスでも、worktree の `.git` ファイルでも、control-plane からは worktree を開けない。

## Decision

- 作業ツリーの変更、途中の操作は、主の worktree と、作業領域の中にある、ほかの worktree のどれかにあれば「ある」とする。
  detached HEAD は、主の worktree とほかの worktree のそれぞれで見て、ほかにないコミットを持つものを加える。
- ほかの worktree は、主のリポジトリの `.git/worktrees/<名前>`（管理ディレクトリ）から見つける。記録されたパスの
  末尾のうち、作業領域の下にあって、その `.git` ファイルが同じ管理ディレクトリを指す最も長いものを、その worktree の
  ファイルの在りかとする。git には `--git-dir`（管理ディレクトリ）と `--work-tree`（見つけた在りか）で渡す。
- 記録はエージェントが書けるので、`..` を含む末尾と、シンボリックリンクをたどると作業領域の外に出る在りかは使わない。
  外の場所を `--work-tree` にして control-plane の git に読ませないため。
- 在りかが見つからない worktree（ファイルを消したもの、作業領域の外にあるもの）は、作業ツリーの変更と途中の操作を
  見ない。detached HEAD は管理ディレクトリだけで読めるので見る。
- 残す理由と、その知らせの文面は変えない（どれも「作業領域に」と書いてあり、ほかの worktree も作業領域の中にある）。

## Alternatives

- **`git worktree list --porcelain` のパスを使う**：コンテナから見たパスなので、control-plane では開けない。
- **コンテナの中のパス（`/workspace/<名前>`）を、作業領域のアダプタが知って読み替える**：パスの形はコンテナを作る
  アダプタ（docker）の決めごとで、作業領域のアダプタがそれを前提にすることになる。
- **worktree を相対パスで記録させる（`worktree.useRelativePaths`）**：git 2.48 から。control-plane のイメージと
  ターゲットのイメージの git（Debian trixie は 2.47）では使えず、ターゲットのイメージにも条件を課す。
- **ほかの worktree は見ずに、締めの手順でエージェントが消す**：締めを頼まれないまま保持期間が過ぎたセッションでは
  失われる。判定を一か所に置くことにも合わない（#220）。

## Consequences

- 変更を残したサブエージェントの worktree があるセッションは、その変更を片付けるまで残る。
- 作業領域の中のほかの場所にコピーした worktree などで、`.git` ファイルが同じ管理ディレクトリを指すものが二つあると、
  長い方の末尾で見つかったものだけを見る。
- worktree を作業領域の外に作ったときは、その作業ツリーはコンテナの書き込み層にあり、片付けを待たずにコンテナを
  消すと失われる。control-plane からは読めないので、見ない。管理ディレクトリにある detached HEAD のコミットは見る。
