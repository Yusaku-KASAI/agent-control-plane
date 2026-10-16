# harness の全体像

## 構成

```
harness/
  hooks/hooks.json     SessionStart と SubagentStart で scripts/context を、PreToolUse で scripts/foreground と scripts/detach を呼ぶ
  scripts/context      動き方の要約と、ハーネスの契約と platform の文書の一覧の在りかを返す
  scripts/foreground   control-plane 経由のとき、サブエージェントをフォアグラウンドで動かす
  scripts/detach       control-plane 経由のとき、処理を手放す Bash と、Monitor、ScheduleWakeup、CronCreate を断る
  tests/               scripts/foreground と scripts/detach の試験
  skills/harness/      詰まったときに、どこの問題かを切り分ける手順
```

- 始めに渡すのは、動き方の短い要約と行き先だけ。渡されるものとその制約は[ハーネスの契約](../../../docs/reference/harness.md)に、
  道具を使う切り分けの手順はスキルに置き、要るときに読ませる。
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

### `scripts/foreground`

- **責務**：control-plane 経由のとき、サブエージェントをフォアグラウンドで動かす（[ADR 0008](../../../docs/adr/0008-subagents-in-foreground-under-control-plane.md)）。
- **置くもの / 置かないもの**：Agent の呼び出しの `run_in_background` を `false` にし、`isolation: remote` の Agent と
  `SendMessage`（宛先によらない）を断る。devcontainer で直接動くときと、ほかのツール（バックグラウンドの Bash など）には
  何もしない。
- **関係**：動き方は契約の見分け方（`ACP_SESSION_BRANCH`）で決める。
- **当てはめた原則・パターン**：[仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)：依頼への回答が一度で済むことを、
  エージェントへの指示ではなく hook で守る。

### `scripts/detach`

- **責務**：control-plane 経由のとき、終わりの知らせが届かない形でバックグラウンドの処理を始めることと、終わりを
  見張ることを断る（[ADR 0015](../../../docs/adr/0015-refuse-let-go-processes-under-control-plane.md)）。
- **置くもの / 置かないもの**：Bash のコマンドを左から一度だけ読み、ヒアドキュメントの中身、引用符の中、コメントを
  除いた残りを見て、演算子の `&`（`&&`、`|&`、リダイレクトの `&` は除く）か、`nohup`、`disown`、`setsid` があれば
  断る。最後の `&` より後ろに、今のシェルで動くコマンドとしての `wait`（パイプや `( )` の中は除く）があるものは
  通す。Monitor、ScheduleWakeup、CronCreate は
  断る。シェルの文法をすべては解かないので、
  まれに通すべきものを断る（断る文で `run_in_background` か `wait` を案内する）。devcontainer で直接動くときは
  何もしない。
- **関係**：動き方は契約の見分け方（`ACP_SESSION_BRANCH`）で決める。CI の待ちは github プラグインの
  `prctl ci --wait` を `run_in_background` で始める。
- **当てはめた原則・パターン**：[仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)：決まりだけでは二度破られたので、
  hook で断る。

### `skills/harness/`

- **責務**：認証、push、マウント、環境変数などで詰まったときに、ハーネスの問題か、権限の問題か、ターゲットの問題かを
  切り分けて報告させる。
- **置くもの / 置かないもの**：確かめる順と、報告の仕方。渡されるものの一覧と条件は置かず、契約を指す。
- **関係**：roles の developer が、push や PR の作成ができないときに使う。
- **当てはめた原則・パターン**：[単一の情報源](../../principles/docs/reference/principles.md#ssot)：渡されるものは契約にだけ書き、スキルは
  それと突き合わせる。
