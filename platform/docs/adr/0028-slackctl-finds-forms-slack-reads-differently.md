---
status: accepted
date: 2026-10-25
---

# 0028. Slack で崩れる Markdown の形は、slack プラグインの slackctl が見つけて知らせる

## Context

Slack の markdown ブロックは、標準の Markdown を少し違う規則で読む（slack プラグインの参照）。control-plane は、
Slack に投稿する前に、意味を変えずに機械で直せる形（裸の URL、表の前の空行）だけを直す（control-plane の ADR 0050）。
太字の内側の ASCII の記号、番号付きのリストの項目の下の入れ子、見出しの中の書式、リストの中のコードブロックは、
直せないのでエージェントの書き方に任せていた。

エージェントは、control-plane 経由でも devcontainer で直接でも動く。devcontainer で `slackctl post` で投稿するときは、
control-plane の直しを通らない。依頼者は、動き方による違いをなるべく減らすことを基本の方針にし、機械の検知は
プラグインの側に持たせると決めた。

## Decision

- slack プラグインの `slackctl check` は、長さの検査に加えて、Slack で崩れる形を行ごとに見つけて知らせ、見つかれば
  失敗する。`slackctl post` も同じ検査をし、見つかれば投稿しない。わざとその形で送るときだけ `--force` を付ける。
- 見つけるのは、control-plane が直す形（裸の URL、表の前の空行）も含めた、参照にある崩れる形。実体参照は、`&amp;` の
  ように、わざとそのまま見せることがあり、写してしまう元は `slackctl` が読むときに戻して断つので、見つけない。どの動き方の
  投稿でも同じ検査を通す。
- 直しはしない。書き直すのはエージェント。
- control-plane が投稿する最後の回答は `slackctl` を通らないので、slack スキルが、書式を多く使う回答は答える前に
  `slackctl check` に通すよう書く。
- 見つける処理は Python で書き、プラグインの `scripts/check-markdown` に置く（slackctl は bash と jq だけでは
  行ごとの文脈を持ちにくい）。

## Alternatives

- **control-plane の Go の直しと見つける処理を一つにする**：Markdown の読み分けが一か所になる。ただ、control-plane は
  プラグインを前提にせず、プラグインは control-plane のコードを持てない。control-plane の直す責務と、プラグインの
  知らせる責務は別のものとみなし、読み分けが二か所に分かれることを受け入れる。
- **slackctl post でも control-plane と同じ直しをする**：動き方の差はさらに減るが、直しを二つの言語で保つことになる。
  見つけて知らせれば、エージェントが書き直せる。
- **知らせるだけで、post は止めない**：知らせが出るのは投稿のあとなので、崩れたまま届く。
- **ターンの終わりの hook で最後の回答を検査する**：control-plane 経由の最後の回答も機械で確かめられるが、回答の
  本文を hook が読む口がなく、誤って止めたときの手戻りが大きい。

## Consequences

- devcontainer でも control-plane 経由でも、`slackctl post` の投稿は同じ検査を通る。
- 最後の回答の検査は、エージェントが `slackctl check` を使うかにかかる。
- 見つける処理は行ごとの走査で、太字の `**` は行の中で順に開きと閉じとして組にする。組み方を誤ると、効いている太字を
  知らせることがある（そのときは `--force`）。
- control-plane の直し（Go）と、この見つける処理（Python）は、どちらも Slack の読み方に合わせて保つ。Slack の読み方が
  変わったら両方を見直す。
