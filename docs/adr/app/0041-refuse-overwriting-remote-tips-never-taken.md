---
status: accepted
date: 2026-10-20
amends: [32]
---

# 0041. 作業領域の pre-push フックで、取り込んだことのないリモートの先頭を上書きする push を断る

## Context

一つのブランチに、ほかのセッションや人が push することがある（既存の PR をその PR の head ブランチで続ける、
platform の ADR 0020）。push したコミットを書き換えるときは `--force-with-lease --force-if-includes` にする、という
決まりは、conventions の変更の扱い方に読んで守るものとして置く（#194）。`--force` だけで push すれば、取り込んで
いないほかの書き手のコミットを上書きして消せる（#49）。

一方で、force push そのものは、依頼者が頼んだ書き換えなどに要るので止めない（Issue #61）。

## Decision

- 作業領域の `pre-push`（ADR 0032）で、リモートにある ref を、fast-forward でなく上書きする push を見る。上書きされる
  リモートの先頭が、push する手元の ref の reflog のどの項目からもたどれなければ、push を断る。手元の ref が `HEAD`
  なら、それが指すブランチの reflog を見る。ブランチやタグの名前でないもの（detached HEAD、コミットの値、`HEAD@{1}`
  のような式、remote-tracking の ref）は、見る reflog がないものとして、fast-forward だけを通す。
- これは git の `--force-if-includes` と同じ考えの確かめを、remote-tracking の ref の値ではなくリモートの実際の先頭で、
  push に付けたフラグによらず行うもの。取り込んだリモートの先頭を書き換える force push は通る。
- 確かめは、名前の注意（ADR 0040）のあと、検査（`make check`）の前に、Makefile の有無によらず行う。
- ハーネスの契約の push の節に、断られることと、断られたら取り込んでから push し直すことを書く。

## Alternatives

- **断らずに注意にとどめる**：注意が出たときには上書きは終わっていて、消えたコミットは作業領域のどこにもない
  （取ってきていなければ）。force push を止めないという決め（#61）は、取り込んだうえでの書き換えに当てはまり、
  取り込まずに消すことは含まない。
- **すべての force push を断る**：依頼者が頼んだ書き換えや、自分が push しただけのコミットの書き換えまで止める。
- **リモートの先頭が手元の先頭からたどれるか（fast-forward か）だけを見る**：書き換えた push はどれも断ることになり、
  上と同じ。
- **remote-tracking ブランチの値と比べる（`--force-with-lease` と同じ確かめ）**：バックグラウンドの fetch などで
  取ってきただけのときに通ってしまう。
- **作業領域の git の設定で `push.useForceIfIncludes` を有効にする**：`--force-with-lease` を付けた push にしか効かず、
  `--force` だけの push は通す。
- **HEAD の reflog で見る**：ほかの書き手のコミットを見るために切り替えただけでも、取り込んだことになる。
- **harness の PreToolUse の hook で、Bash の `git push` の字面から `--force` を見る**：フラグの書き方が多く、
  字面からは確かに拾えない（ADR 0032 の Alternatives と同じ）。

## Consequences

- 取ってきていない、または手元のブランチに一度も合わせていないほかの書き手のコミットを、作業領域から上書きして
  消すことはなくなる。手元のブランチを一度リモートの先頭に合わせれば（`reset --hard` で見ただけでも）、取り込んだ
  ものとして通る。`--force-if-includes` と同じ限界で、合わせたあとの書き換えは conventions の決まり（#194）で守る。
- 人がホストで作業領域から push しても、同じように断られる（`--no-verify` で外せる）。
- ブランチやタグの名前でないもの（detached HEAD、コミットの値、式）から push するときは、fast-forward でなければ
  断られる。ブランチに切り替えてから push する。
- 手元のブランチの reflog が消えている（`git reflog expire` など）と、取り込んだはずのものでも断られる。
- ブランチを消す push と、新しい ref を作る push は見ない。タグはふつう reflog がないので、上書きは断られる。
- この版より前に作った作業領域のフックは置き換わらず、確かめは効かない。
- `core.hooksPath` を設定しているターゲットや、`--no-verify` の push では効かない。
