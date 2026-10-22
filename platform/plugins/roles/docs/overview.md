# roles の全体像

## 構成

roles は harness、github と conventions を前提にする。roles を有効にするリポジトリは、この三つも有効にする。
ファイルを変えないロールの隔離は、ターゲットの `.claude/settings.json` の `worktree.baseRef: head` と、`.gitignore` の
`/.claude/worktrees/` を前提にする（[ターゲットの契約](../../../../template/docs/reference/contract.md#plugins "在りか")）。

```
roles/
  agents/developer.md   変更を実装して PR にする。レビューも受け持つ
  agents/guide.md       質問と相談に答え、変更しない
  agents/reviewer.md    レビューし、結果を PR のレビューと Issue に残す。変更しない
  agents/pm.md          課題を整え、相談に乗り、Issue に残す。変更しない
```

- ロールは Claude Code のカスタムエージェント。control-plane は `--agent <ロール>` で起動し（[ハーネスの契約](../../../docs/reference/harness.md "在りか")）、
  対話ではサブエージェントとしても使う。
- ファイルを変えないロール（guide、reviewer、pm）は、定義の frontmatter に `isolation: worktree` を持つ。サブエージェント
  として動くときは自分の worktree で動き、試すための編集もそこでする（[ADR 0016](../../../docs/adr/0016-subagents-leave-the-work-area.md)）。
  隔離するのは作業する場所で、Claude Code（2.1.292 で確かめた）は、隔離されたエージェントのコマンドのうち、字面から
  頼んだエージェントの作業ツリーを指すと分かる git（`cd` してからや `git -C`）と、字面で確かめられない複雑なコマンド
  （ヒアドキュメント、ループなど）を断る。スクリプトのファイルの中の git は見ない。ファイルを読むことはできる。
  git 以外の手段での書き込みまで防ぐかは確かめていない。
  - worktree は頼まれたときの HEAD から作られる（`worktree.baseRef: head`）。ターゲットがこの設定を持たなければ、
    既定のブランチから作られる。頼んだ側の HEAD は、自分の worktree で `git worktree list --porcelain` を流すと、
    頼んだ側の作業ツリーの行に出る（自分の HEAD は `git rev-parse HEAD`）。
  - コミットしていない変更は、自分の worktree には入らない。頼んだ側の作業ツリーのファイルを読むか、差分を渡して
    もらう。
- ロールの定義は常に読み込まれるので、振る舞いだけを短く書き、節に分ける。動き方ごとの事実はハーネスの契約に、
  観点と共通の手順はスキルに任せ、定義からはそれを指す。ロールは権限と隔離の境界で、手順とは多対多になる。定義には、
  書ける範囲と振り分けの目的の手順ごとの扱いを並べる「受け持つ手順」の節を置く（考え方は conventions の
  [目的の手順と共通の手順](../../conventions/docs/architecture/documentation.md#procedures)、定義の形は
  [形式：役割ごとの指示](../../conventions/docs/reference/formats.md#role)）。書ける範囲を認証情報で守れるのは、
  control-plane がそのロールを自分の GitHub App で起動するときに、App の権限で分けられる粒度（Contents、Pull requests、
  Issues の読み書き）までだけ。それより細かい範囲（reviewer のレビューを COMMENT だけにすること、PR を書き換えないこと）と、
  サブエージェントとして動くとき（頼んだ側の認証情報を使う）は、ロールの指示（と、COMMENT だけを付ける `prctl review`）が守る。

## 構成要素

### [`agents/developer.md`](../agents/developer.md "在りか")

- **責務**：ファイルを変え PR を作る権限を持ち、主に依頼された変更をターゲットのリポジトリに実装し、PR にして届ける。すべてに書けるので、レビューするも受け持つ。差分を読むのは reviewer のサブエージェントに頼み、結果を残す（[ADR 0019](../../../docs/adr/0019-roles-by-write-scope.md)）。
- **置くもの / 置かないもの**：「受け持つ手順」の節に、書ける範囲と目的の手順ごとの扱い（どれを受け持つかは指示の節にある）を、
  「使う決まり」の節に使う決まりの要約と案内（変更を届ける手順、書き方、スキル）を、「固有の振る舞い」の
  節に developer だけの定義（PR にすること、報告）を置き、使うモデル（[ADR 0003](../../../docs/adr/0003-roles-choose-their-model.md)）を持つ。
  要約には出典（conventions の定義）を付ける（conventions の起点の決まりに沿う）。ハーネスの事実とターゲット固有の事実は置かない。
- **関係**：conventions の変更を届ける手順と書き方、ハーネスの契約、conventions の design、write-docs、review の各スキル、
  github スキル、harness スキルを前提にする。差分のレビューは reviewer ロールのサブエージェントに頼む。
- **当てはめた原則・パターン**：[一つの出典から届ける](../../principles/docs/reference/patterns.md#single-source)：役割ごとの指示は決まりを要約か案内で載せ、要約には出典を付ける。
  [関心の分離](../../principles/docs/reference/principles.md#soc)：事実は契約とターゲットの CLAUDE.md に任せる。

### [`agents/guide.md`](../agents/guide.md "在りか")

- **責務**：ターゲットのリポジトリの仕組みや使い方についての質問と相談に、ファイルを変更せずに答える。
- **置くもの / 置かないもの**：受け持つ手順（書ける範囲は GitHub に何も書かない。受け持たない手順は回す先を案内する。
  どれを受け持つかは指示の節にある）と、答え方と、しないこと。developer と同じく、ハーネスの事実とターゲット固有の事実は置かない。
- **関係**：書けないことが要る依頼は、書けるロールに回すよう案内する（回す先は指示の「受け持つ手順」にある）。
- **当てはめた原則・パターン**：[関心の分離](../../principles/docs/reference/principles.md#soc)。

### [`agents/reviewer.md`](../agents/reviewer.md "在りか")

- **責務**：変更した人が頼む差分のレビュー、PR の差分のレビュー、全体のレビューを、ファイルを変更せずに行い、結果を
  頼んだ人に返すか、PR のレビューと Issue に残す（[ADR 0018](../../../docs/adr/0018-reviewer-reviews-own-changes-too.md)）。
- **置くもの / 置かないもの**：「受け持つ手順」の節に、書ける範囲と目的の手順ごとの扱い（どれを受け持つかは指示の節に
  ある）を、「使う決まり」の節に、レビューの
  決まり（PR 全体を見る、残し方、観点、指摘）の要約と案内を置き、要約には出典（conventions のレビューする手順と
  レビューの扱い方）を付ける。「固有の振る舞い」の節に、何をするか（差分と全体の見分け方と、developer との分担）、
  しないこと（変更、承認、変更の要求）、報告を置く。何をするかは、種類（差分か全体か）と残し方（返すか
  残すか）の二つで分ける。
- **関係**：conventions のレビューする手順とレビューの扱い方、design スキル、principles スキル、github スキルの
  「ほかの人の PR と既定のブランチを読む」を前提にする。guide と分け、書き込める権限（PR のレビューと Issue）を持つ（[ADR 0007](../../../docs/adr/0007-reviewer-role.md)）。
- **当てはめた原則・パターン**：[別の目で見る](../../principles/docs/reference/patterns.md#four-eyes)：変更した
  エージェントとは別のエージェントがレビューする。[単一責任](../../principles/docs/reference/principles.md#srp)：答えることと
  判定して残すことを分ける。

### [`agents/pm.md`](../agents/pm.md "在りか")

- **責務**：課題（Issue とロードマップ）を整え、相談や壁打ちに乗り、考えとやることを Issue に残す。書ける範囲は Issue
  だけ（[ADR 0022](../../../docs/adr/0022-pm-role.md)）。
- **置くもの / 置かないもの**：「受け持つ手順」の節に、書ける範囲と目的の手順ごとの扱い（どれを受け持つかは指示の節に
  ある）を、「使う決まり」の節に、主に使う手順の要約と案内を置き、要約には出典（conventions の手順と課題の扱い方）を付ける。
  「固有の振る舞い」の節に、しないこと（変更、PR、PR のレビュー）と報告を置く。
- **関係**：conventions の課題を整えると相談に乗るの手順、課題の扱い方、principles スキルを前提にする。書けないことが
  要る依頼は、書けるロールに回す（回す先は指示の「受け持つ手順」にある）。
- **当てはめた原則・パターン**：[最小権限](../../principles/docs/reference/principles.md#least-privilege)：Issue を整えるのに
  要る書き込み（Issue）だけを持つ。[関心の分離](../../principles/docs/reference/principles.md#soc)：課題を整えることと、
  変えることを分ける。
