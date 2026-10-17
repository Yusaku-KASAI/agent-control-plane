# roles の全体像

## 構成

roles は harness、github と conventions を前提にする。roles を有効にするリポジトリは、この三つも有効にする。
ファイルを変えないロールの隔離は、ターゲットの `.claude/settings.json` の `worktree.baseRef: head` と、`.gitignore` の
`/.claude/worktrees/` を前提にする（[ターゲットの契約](../../../../template/docs/reference/contract.md#plugins "在りか")）。

```
roles/
  agents/developer.md   変更を実装して PR にする
  agents/guide.md       質問に答え、変更しない
  agents/reviewer.md    レビューし、結果を PR のレビューと Issue に残す。変更しない
```

- ロールは Claude Code のカスタムエージェント。control-plane は `--agent <ロール>` で起動し（[ハーネスの契約](../../../docs/reference/harness.md "在りか")）、
  対話ではサブエージェントとしても使う。
- ファイルを変えないロール（guide、reviewer）は、定義の frontmatter に `isolation: worktree` を持つ。サブエージェント
  として動くときは自分の worktree で動き、試すための編集もそこでする（[ADR 0016](../../../docs/adr/0016-subagents-leave-the-work-area.md)）。
  隔離するのは作業する場所で、Claude Code（2.1.292 で確かめた）は、隔離されたエージェントが頼んだエージェントの
  作業ツリーで git を動かすこと（`cd` してからや `git -C`）と、git を含むつないだコマンドを断る。ファイルを読むことは
  できる。git 以外の手段での書き込みまで防ぐかは確かめていない。worktree は HEAD から作られる
  （`worktree.baseRef: head`）ので、コミットしていない変更は、頼んだエージェントの作業ツリーのファイルを読むか、
  差分を渡して見る。
- ロールの定義は常に読み込まれるので、振る舞いだけを短く書く。動き方ごとの事実はハーネスの契約に、観点と手順は
  スキルに任せ、定義からはそれを指す。

## 構成要素

### [`agents/developer.md`](../agents/developer.md "在りか")

- **責務**：依頼された変更をターゲットのリポジトリに実装し、PR にして届ける。
- **置くもの / 置かないもの**：「使う決まり」の節に使う決まりの要約と案内（変更の進め方、書き方、スキル）を、「固有の振る舞い」の
  節に developer だけの定義（PR にすること、報告）を置き、使うモデル（[ADR 0003](../../../docs/adr/0003-roles-choose-their-model.md)）を持つ。
  要約には出典（conventions の定義）を付ける（conventions の起点の決まりに沿う）。ハーネスの事実とターゲット固有の事実は置かない。
- **関係**：conventions の変更の扱い方と書き方、ハーネスの契約、conventions の design スキル、github スキル、harness スキルを
  前提にする。
- **当てはめた原則・パターン**：[一つの出典から届ける](../../principles/docs/reference/patterns.md#single-source)：役割ごとの指示は決まりを要約か案内で載せ、要約には出典を付ける。
  [関心の分離](../../principles/docs/reference/principles.md#soc)：事実は契約とターゲットの CLAUDE.md に任せる。

### [`agents/guide.md`](../agents/guide.md "在りか")

- **責務**：ターゲットのリポジトリの仕組みや使い方についての質問に、ファイルを変更せずに答える。
- **置くもの / 置かないもの**：答え方と、しないこと。developer と同じく、ハーネスの事実とターゲット固有の事実は置かない。
- **関係**：変更が要る依頼は developer に回すよう案内する。
- **当てはめた原則・パターン**：[関心の分離](../../principles/docs/reference/principles.md#soc)。

### [`agents/reviewer.md`](../agents/reviewer.md "在りか")

- **責務**：PR の差分のレビューと全体のレビューを、ファイルを変更せずに行い、結果を PR のレビューと Issue に残す。
- **置くもの / 置かないもの**：「使う決まり」の節に、レビューの決まり（PR 全体を見る、残し方、観点、指摘）の要約と案内を
  置き、要約には出典（conventions のレビューの扱い方）を付ける。「固有の振る舞い」の節に、何をするか（差分と全体の
  見分け方と、developer との分担）、しないこと（変更、承認、変更の要求）、報告を置く。
- **関係**：conventions のレビューの扱い方と review スキル、design スキル、principles スキル、github スキルの
  「ほかの人の PR と既定のブランチを読む」を前提にする。guide と分け、書き込める権限（PR のレビューと Issue）は reviewer
  だけが持つ（[ADR 0007](../../../docs/adr/0007-reviewer-role.md)）。
- **当てはめた原則・パターン**：[別の目で見る](../../principles/docs/reference/patterns.md#four-eyes)：変更した
  エージェントとは別のエージェントがレビューする。[単一責任](../../principles/docs/reference/principles.md#srp)：答えることと
  判定して残すことを分ける。
