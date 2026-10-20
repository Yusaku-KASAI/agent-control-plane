---
status: accepted
date: 2026-10-20
amends: [32, "../../../platform/docs/adr/0020-continue-a-pr-on-its-head-branch.md"]
---

# 0041. 作業領域の pre-push フックで、取り込んだことのないリモートの先頭を上書きする push を注意して止め、承知を示せば通す

## Context

一つのブランチに、ほかのセッションや人が push することがある（既存の PR をその PR の head ブランチで続ける、
platform の ADR 0020）。`--force` で push すると、取り込んでいないほかの書き手のコミットを上書きして消せる（#49）。

ただ、force push で履歴を書き換えることや、ほかの書き手の push を消すことは、したいときもあり、人でも起こる。
force push は止めないと決めている（platform の ADR 0020、Issue #61）。困るのは、気づかずに消すことだけ。

## Decision

- 作業領域の `pre-push`（ADR 0032）で、リモートにある ref を fast-forward でなく上書きする push を見る。上書き
  されるリモートの先頭が、push する手元の ref の reflog のどの項目からもたどれなければ、`acp pre-push: 注意` で
  始まる注意を出して push を止める。手元の ref が `HEAD` なら、それが指すブランチの reflog を見る。ブランチや
  タグの名前でないもの（detached HEAD、コミットの値、`HEAD@{1}` のような式）は、見る reflog がないものとする。
- 環境変数 `ACP_PUSH_OVERWRITE=1` を付けた push は止めない。上書きしてよいと判断したら、それを付けて push し直す。
- 確かめは、名前の注意（ADR 0040）のあと、検査（`make check`）の前に、Makefile の有無によらず行う。
- ハーネスの契約の push の節に、止まることと、承知を示して push し直す方法を書く。

## Alternatives

- **断る（承知を示しても通さない）**：force push で消したいときもあり、人でも同じことをする。止めないと決めた
  force push を、仕組みで禁じることになる。
- **注意だけ出して止めない**：注意が出たときには上書きは終わっていて、取ってきていないコミットは作業領域のどこにも
  ない。一度止めて、承知したうえで push し直せば、気づかずに消すことだけを防げる。
- **conventions に、push の合わせ方と `--force-with-lease --force-if-includes` を細かく決める**：場面ごとの決まりが
  増え、それでも読むことに頼る。気づかせる仕組みを一つ置く方が単純。
- **`--force-with-lease` だけに頼る**：取ってきただけ（バックグラウンドの fetch など）で通る。`--force` だけの push
  には効かない。
- **HEAD の reflog で見る**：ほかの書き手のコミットを見るために切り替えただけでも、取り込んだことになる。

## Consequences

- 取ってきていない、または手元のブランチに一度も合わせていないほかの書き手のコミットを上書きする push は、一度
  止まる。手元のブランチを一度リモートの先頭に合わせれば（`reset --hard` で見ただけでも）、取り込んだものとして
  通る（`--force-if-includes` と同じ限界）。
- ブランチやタグの名前でないものから push するときは、fast-forward でなければ止まる。
- 人がホストで作業領域から push しても、同じように止まる。
- ブランチを消す push と、新しい ref を作る push は見ない。
- この版より前に作った作業領域のフックは置き換わらず、確かめは効かない。
- `core.hooksPath` を設定しているターゲットや、`--no-verify` の push では効かない。
