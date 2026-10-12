# harness の全体像

## 構成

```
harness/
  hooks/hooks.json     SessionStart と SubagentStart で scripts/context を呼ぶ
  scripts/context      動き方の要約と、ハーネスの契約と platform の文書の一覧の在りかを返す
  skills/harness/      詰まったときに、どこの問題かを切り分ける手順
```

- 始めに渡すのは、動き方の短い要約と行き先だけ。渡されるものとその制約は[ハーネスの契約](../../../docs/reference/harness.md)に、
  切り分けの手順はスキルに置き、要るときに読ませる。
- 動き方の見分け方は契約にある。hook もスキルもそれに従う。

## 構成要素

### `hooks/hooks.json`、`scripts/context`

- **責務**：セッションとサブエージェントの始めに、動き方の要約と、ハーネスの契約と platform の文書の一覧の在りかを
  伝える。
- **置くもの / 置かないもの**：動き方ごとの数行の要約と、文書のパス。要約は契約の要約と分かるようにし、契約にない
  事実や細部（環境変数の一覧、ブランチの分け方など）は書かない。
- **関係**：文書のパスは、マーケットプレイスの中でプラグインの隣にある `docs/` から決める。プラグインが別の場所に
  写されていて見つからなければ、ターゲットがマーケットプレイスを宣言する `/workspace/agent-platform/docs` を使う。
- **当てはめた原則・パターン**：[使う側に届く契約](../../principles/docs/reference/patterns.md#published-language)：control-plane のリポジトリを読めない
  エージェントにも、契約の在りかを届ける。[段階的な開示](../../principles/docs/reference/principles.md#progressive-disclosure)：常に読み込まれるので、
  中身ではなく行き先を渡す。

### `skills/harness/`

- **責務**：認証、push、マウント、環境変数などで詰まったときに、ハーネスの問題か、権限の問題か、ターゲットの問題かを
  切り分けて報告させる。
- **置くもの / 置かないもの**：確かめる順と、報告の仕方。渡されるものの一覧と条件は置かず、契約を指す。
- **関係**：roles の developer が、push や PR の作成ができないときに使う。
- **当てはめた原則・パターン**：[単一の情報源](../../principles/docs/reference/principles.md#ssot)：渡されるものは契約にだけ書き、スキルは
  それと突き合わせる。
