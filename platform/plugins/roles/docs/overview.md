# roles の全体像

## 構成

roles は harness、github と conventions を前提にする。roles を有効にするリポジトリは、この三つも有効にする。

```
roles/
  agents/developer.md   変更を実装して PR にする
  agents/guide.md       質問に答え、変更しない
  agents/reviewer.md    レビューし、結果を PR のレビューと Issue に残す。変更しない
```

- ロールは Claude Code のカスタムエージェント。control-plane は `--agent <ロール>` で起動し（[ハーネスの契約](../../../docs/reference/harness.md "案内")）、
  対話ではサブエージェントとしても使う。
- ロールの定義は常に読み込まれるので、振る舞いだけを短く書く。動き方ごとの事実はハーネスの契約に、観点と手順は
  スキルに任せ、定義からはそれを指す。

## 構成要素

### [`agents/developer.md`](../agents/developer.md "案内")

- **責務**：依頼された変更をターゲットのリポジトリに実装し、PR にして届ける。
- **置くもの / 置かないもの**：developer に固有の振る舞い（PR にすること、報告）と、使う決まりの選び出し（変更の進め方、書き方、
  スキル）、使うモデル（[ADR 0003](../../../docs/adr/0003-roles-choose-their-model.md)）。選び出した決まりは要約だけを書き、
  出典（conventions の定義）を示す（conventions の届け方の決まりに沿う）。ハーネスの事実とターゲット固有の事実は置かない。
- **関係**：conventions の変更の扱い方と書き方、ハーネスの契約、conventions の design スキル、github スキル、harness スキルを
  前提にする。
- **当てはめた原則・パターン**：[一つの出典から届ける](../../principles/docs/reference/patterns.md#single-source)：役割ごとの指示は決まりの写しで、出典を示す。
  [関心の分離](../../principles/docs/reference/principles.md#soc)：事実は契約とターゲットの CLAUDE.md に任せる。

### [`agents/guide.md`](../agents/guide.md "案内")

- **責務**：ターゲットのリポジトリの仕組みや使い方についての質問に、ファイルを変更せずに答える。
- **置くもの / 置かないもの**：答え方と、しないこと。developer と同じく、ハーネスの事実とターゲット固有の事実は置かない。
- **関係**：変更が要る依頼は developer に回すよう案内する。
- **当てはめた原則・パターン**：[関心の分離](../../principles/docs/reference/principles.md#soc)。

### [`agents/reviewer.md`](../agents/reviewer.md "案内")

- **責務**：PR の差分のレビューと全体のレビューを、ファイルを変更せずに行い、結果を PR のレビューと Issue に残す。
- **置くもの / 置かないもの**：差分と全体のどちらをするかの見分け方、しないこと（変更、承認、変更の要求）、結果の残し方、
  報告。レビューの決まりは要約だけを書き、出典（conventions のレビューの扱い方）を示す。
- **関係**：conventions のレビューの扱い方と review スキル、design スキル、principles スキル、github スキルの
  「ほかの人の PR と既定のブランチを読む」を前提にする。guide と分け、書き込める権限（PR のレビューと Issue）は reviewer
  だけが持つ（[ADR 0007](../../../docs/adr/0007-reviewer-role.md)）。
- **当てはめた原則・パターン**：[別の目で見る](../../principles/docs/reference/patterns.md#four-eyes)：変更した
  エージェントとは別のエージェントがレビューする。[単一責任](../../principles/docs/reference/principles.md#srp)：答えることと
  判定して残すことを分ける。
