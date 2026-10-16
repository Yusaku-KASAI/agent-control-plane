---
status: accepted
date: 2026-10-17
---

# 0016. control-plane 経由では、サブエージェントに作業領域を変えさせない

## Context

レビューのサブエージェントが試しに動かしたコマンドが、作業領域の一番上にファイルを作った。セッションの
エージェントの `git add -A` でそれがコミットに入り、依頼者が見つけた。レビューのサブエージェントが作業領域を
detached HEAD にしたことも 2 回あった（Issue #141 の A5、Issue #143）。reviewer ロールは「作業領域のファイルと HEAD を
変えない」と決めているが、頼むたびに依頼文で念を押していた。

作業領域でコミットし push するのはセッションのエージェントで、そのコミットは作業ツリーの中身から作る。サブエージェントが
残した変更は、エージェントが自分の変更と見分けないかぎりコミットに紛れる。

Claude Code の hook の入力は、サブエージェントのツールの呼び出しにだけ `agent_id` を持つ（セッションのエージェントの
呼び出しには `agent_type` だけがある。2.1.292 で確かめた）。

## Decision

- harness プラグインに hook（`scripts/guard-tree`）を置き、control-plane 経由のときは次をする。
  - **断る**（PreToolUse）：サブエージェントの Edit、Write、NotebookEdit のうち、作業領域（主の worktree）の中に
    書くもの。サブエージェントの Bash のうち、作業領域の中で、作業ツリー、index、HEAD を変える git のサブコマンドを
    動かすもの。サブコマンドは `add`、`am`、`apply`、`bisect`、`checkout`、`checkout-index`、`cherry-pick`、`clean`、
    `commit`、`format-patch`、`merge`、`mv`、`pull`、`read-tree`、`rebase`、`reset`、`restore`、`revert`、`rm`、
    `sparse-checkout`、`stash`、`submodule`、`switch`、`symbolic-ref`、`update-index`、`update-ref` で、読むだけの形
    （`stash list`、`bisect log`、`apply --check`、`symbolic-ref HEAD`、`format-patch --stdout`、`submodule status`
    など）は通す。git の場所は、今のディレクトリ（同じコマンドの中の `cd` を追う）か、`-C`、`--git-dir`、
    `--work-tree` が指す所。引用符の中や変数の展開で書かれ、字面から読めない場所は通し、次の比べに任せる。
    コマンドは detach と同じく字面で読む。
  - **知らせる**（Agent の PreToolUse と、PostToolUse、PostToolUseFailure）：セッションのエージェントがサブエージェントを
    動かす前後で、作業領域の HEAD、ブランチ、index、stash、変更のあるファイルと追跡していないファイル（ignore
    されたものは除く）の中身を比べ、変わっていれば何が変わったかをエージェントに知らせる。リダイレクトやスクリプトの
    中など、字面では見つけられない書き換えはここで拾う。
- linked worktree（`prctl worktree` の一時的な worktree、`isolation: worktree` のサブエージェント）と、作業領域の外
  （`mktemp -d` の場所）には何もしない。devcontainer で直接動くときも何もしない。
- review の手順に、範囲のファイルを `git diff --stat` で先に確かめることと、作業領域を変えず、試すなら一時的な場所を
  使うことを書く。変更の進め方に、コミットの前に `git status` で入れるファイルを確かめることを書く。
- シェルの字面の読み方（`shell_words`）は、detach と共有するので `lib/shell.sh` に移す。

## Alternatives

- **決まりと依頼文の念押しのままにする**：決まりはあって、破られた（仕組みで守る）。
- **断るだけにする**：リダイレクト（`> file`）やスクリプトの中の書き換えは、字面で見つけきれない。実際に作られた
  ファイルも、壊れたコマンドのリダイレクトからだった。比べる形なら、手段によらず見つかる。
- **知らせるだけにする**：書き換えたあとに戻すことになる。よく起きる形（Edit や Write、`git checkout`）は、
  起きる前に断る方が安い。
- **サブエージェントの終わり（SubagentStop）で断り、サブエージェント自身に戻させる**：変更を見分けて戻すのは、
  作業領域の持ち主のエージェントの方が確か。止め続けて終わらないこともありうる。
- **レビューは必ず `prctl worktree` の一時的な worktree で行わせる**：自分の変更のレビューは作業領域の HEAD を読む
  のでそのままで足り、worktree で読ませても、作業領域へ書くことは止められない。
- **devcontainer で直接動くときも断る**：人が端末にいて、サブエージェントに作業領域を変えさせたいこともある。
  そこでは決まり（reviewer ロールの「しないこと」）に任せる。

## Consequences

- control-plane 経由では、サブエージェントは作業領域のファイルを書けず、作業領域で HEAD、index、作業ツリーを変える
  git のコマンドを使えない。作業を任せるなら `isolation: worktree` にするか、セッションのエージェントが自分で行う。
- 字面で読むので、`echo git checkout` のように git のコマンドでないものを断ることがある。逆に、場所を引用符や変数で
  書いた git のコマンド（`git -C "$dir" checkout`）や、`bash -c` やスクリプトの中の git のコマンドは断らず、前後の
  比べで知らせるだけになる。
- Agent がほかの hook に断られるなどして、あとの hook が呼ばれないと、写しが残る。1 日より古い写しは、次の Agent の
  前に消す。
- 比べるのは前後の状態なので、サブエージェントが変えてから戻したものは知らせない。並べて動かしたサブエージェントの
  どれが変えたかは分からない。比べるための写しを作業領域の `.git` に一時的に置く。
