---
status: accepted
date: 2026-10-17
amends: [6]
---

# 0017. CI の状態は、同じ app の同じ名前の check run のうち一番新しいものだけで判定する

## Context

ADR 0006 で、CI の状態は「各 check の最新の run（`filter=latest`）」で判定すると決めた。`filter=latest` は、
check suite ごとに最新の check run に絞るだけで、同じ head に check suite が二つ以上あると、古い suite の check run も
返す。

PR #149 で、head の最初の run は GitHub の支払いの制限でジョブが起動せず、すべて failure になった。この run は
再実行できなかったので、PR を閉じて開き直し、同じ head で新しい run（新しい check suite）を流した。新しい run は
すべて success だったが、`prctl ci` は古い suite の failure を数えて failed と判定し、ターンの終わりの hook も止め
続けた（Issue #153）。同じ head に suite が増えるのは、PR を開き直したとき、ワークフローを手で流したときなど。

## Decision

- `filter=latest` で読んだ check run を、app と名前ごとにまとめ、`id` が一番大きいものだけを残してから、ADR 0006 の
  分け方で判定する。GitHub の check run の `id` は、あとから作られたものほど大きい（API の説明にはなく、観察による）。
  まとめる鍵を app と名前だけにするのは、check run の答えだけで決められ、ワークフローを引かずに済むから。
- 名前だけでなく app でもまとめる。ほかの app の同じ名前の check run が、失敗を隠さないようにする。
- 偽の gh が返す check run にも、本物と同じく `id` と `app` を持たせ、二つの suite にまたがる場合を試験する。

## Alternatives

- **check suite ごとに読み、一番新しい suite だけを見る**：ワークフローが二つ以上あると、それぞれが別の suite に
  なる。一番新しい suite だけを見ると、ほかのワークフローの失敗を見落とす。
- **`started_at` で新しさを決める**：起動しなかった run や、まだ始まっていない run の扱いが要る。`id` は、どの
  状態の check run にもある。
- **古い run の失敗は人が消す**：check run は消せず、再実行できない run もある（今回がそうだった）。
- **GraphQL の `statusCheckRollup` で、ワークフローとイベントまで鍵にする**：check run から check suite、
  ワークフローの run をたどり、ワークフローの名前とイベントも一度の問い合わせで取れる（フィールドは知識によるもので、
  問い合わせて確かめてはいない）。別のワークフローの同じ名前を分けられる。このリポジトリのワークフローは
  一つで、push で流すのは既定のブランチだけなので、PR の head で同じ名前がぶつからない（ほかのターゲットは
  確かめていない）。読み出しを REST から GraphQL に
  変え、偽の gh もそれに合わせることになるので、別のワークフローの同じ名前が要る場面が出るまでは選ばない。

## Consequences

- 同じ head で CI を流し直せば、古い失敗は判定から外れる。逆に、通ったあとの流し直しで失敗すれば、failed になる。
- 同じ app の同じ名前の check run は、別のワークフローのものでも、別のイベント（push と pull_request など）の
  ものでも、一つとして扱い、新しい方だけを見る。古い方の失敗は、新しい方が通れば判定から外れる。分けるには、
  ワークフローまで読む（Alternatives の GraphQL）必要があり、今はそうしない。
- `id` が作られた順に大きくなることは、GitHub の API の説明にはなく、これまでの観察によるもの。
