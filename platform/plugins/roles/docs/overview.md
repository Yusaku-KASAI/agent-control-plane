# roles の全体像

## 構成

roles は harness と conventions を前提にする。roles を有効にするリポジトリは、この二つも有効にする。

```
roles/
  agents/developer.md   変更を実装して PR にする
  agents/guide.md       質問に答え、変更しない
```

- ロールは Claude Code のカスタムエージェント。control-plane は `--agent <ロール>` で起動し（[ハーネスの契約](../../../docs/reference/harness.md)）、
  対話ではサブエージェントとしても使う。
- ロールの定義は常に読み込まれるので、振る舞いだけを短く書く。動き方ごとの事実はハーネスの契約に、観点と手順は
  スキルに任せ、定義からはそれを指す。

## 構成要素

### `agents/developer.md`

- **責務**：依頼された変更をターゲットのリポジトリに実装し、PR にして届ける。
- **置くもの / 置かないもの**：進め方（前提を確かめる、壊れうる場面を挙げる、レビューさせる）と、報告に書くこと、
  使うモデル（[ADR 0003](../../../docs/adr/0003-roles-choose-their-model.md)）。作業するブランチ、依頼者に聞いて待てるかなどの
  ハーネスの事実と、ターゲット固有の事実は置かない。
- **関係**：ハーネスの契約、conventions の design スキル、github スキル、harness スキルを使う。
- **当てはめた原則・パターン**：[関心の分離](../../principles/docs/reference/principles.md#soc)：振る舞いだけを持ち、事実は契約とターゲットの
  CLAUDE.md に任せる。[段階的な開示](../../principles/docs/reference/principles.md#progressive-disclosure)：観点や手順の中身はスキルを参照する。

### `agents/guide.md`

- **責務**：ターゲットのリポジトリの仕組みや使い方についての質問に、ファイルを変更せずに答える。
- **置くもの / 置かないもの**：答え方と、しないこと。developer と同じく、ハーネスの事実とターゲット固有の事実は置かない。
- **関係**：変更が要る依頼は developer に回すよう案内する。
- **当てはめた原則・パターン**：[関心の分離](../../principles/docs/reference/principles.md#soc)。
