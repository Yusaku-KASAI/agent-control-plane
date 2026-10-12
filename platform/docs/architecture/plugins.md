# プラグイン

platform の各プラグインの責務と、プラグインの間の関係。各プラグインの中の構成と構成要素は、そのプラグインの文書に
ある。

## 構成

```
plugins/
  harness/   動き方と、ハーネスの契約の在りかを伝える
  roles/     ロール（developer、guide）
  github/    PR の本文を今のブランチに合わせ続ける
  slack/     Slack を読み書きする
  bell/      対話で止まったら端末のベルを鳴らす
```

```
roles ──前提にする──▶ harness（動き方と契約の在りか）
  │   ──前提にする──▶ conventions（決まり、設計の観点）──前提にする──▶ principles（原則とパターン）
  └─使う──▶ github、slack のスキル
github、slack ──使う──▶ ハーネスの契約で渡される認証情報
bell（対話のときのベル）は独立
```

- 各プラグインは `plugins/<名前>/` にあり、hook、スキル、コマンド（`bin/`）、スクリプト、カスタムエージェント
  （`agents/`）のうち要るものを持つ。文書の根はそれぞれの `docs/`。
- マーケットプレイスは、platform のプラグインのほかに、principles と conventions のプラグインも運ぶ。この二つは
  platform に属さない別の区切りで、platform のプラグインはそれを前提にしてよい（[principles の文書](../../plugins/principles/docs/README.md)、
  [conventions の文書](../../plugins/conventions/docs/README.md)）。

## 構成要素

### `harness`

- **責務**：エージェントに、どう動かされているかと、ハーネスの契約と platform の文書の一覧の在りかを伝え、詰まったときの
  切り分けを助ける。
- **置くもの / 置かないもの**：短い案内と、切り分けの手順。契約の中身は[ハーネスの契約](../reference/harness.md)に置く。
- **関係**：ほかのプラグインに依存しない。roles が前提にする。中の構成は [harness の文書](../../plugins/harness/docs/README.md)。
- **当てはめた原則・パターン**：[使う側に届く契約](../../plugins/principles/docs/reference/patterns.md#published-language)：control-plane のリポジトリを読めない
  エージェントにも、契約を届ける。

### `roles`

- **責務**：共通のロール（Claude Code のカスタムエージェント）を定義する。
- **置くもの / 置かないもの**：ロールの振る舞いと、そのロールが使うモデル（[ADR 0003](../adr/0003-roles-choose-their-model.md)）。
  ターゲット固有の事実や、ハーネスの事実は置かない。
- **関係**：harness が伝える動き方とハーネスの契約、conventions の決まりと観点を前提にする。github と slack のスキルを
  使う。中の構成は [roles の文書](../../plugins/roles/docs/README.md)。
- **当てはめた原則・パターン**：[関心の分離](../../plugins/principles/docs/reference/principles.md#soc)：振る舞いだけを持ち、事実は契約とターゲットの
  CLAUDE.md に任せる。

### `github`

- **責務**：PR の本文を今のブランチに合わせ続け、GitHub での作業（PR、レビュー、CI）の手順を届ける
  （[ADR 0002](../adr/0002-pr-body-tracks-head.md)）。
- **関係**：`gh` の認証を使う（エージェントへの渡し方は[ハーネスの契約](../reference/harness.md)）。roles の developer が
  使う。中の構成は [github の文書](../../plugins/github/docs/README.md)。
- **当てはめた原則・パターン**：[仕組みで守る](../../plugins/principles/docs/reference/principles.md#mechanisms)：本文が古いことを hook が見つけ、
  書き換えを促す。

### `slack`

- **責務**：Slack のメッセージとスレッドを読み、投稿する。
- **関係**：渡された認証情報の組み合わせで、できることが決まる（エージェントへの渡し方は
  [ハーネスの契約](../reference/harness.md)）。中の構成は [slack の文書](../../plugins/slack/docs/README.md)。

<a id="bell"></a>

### `bell`

- **責務**：対話で使っている Claude Code が止まったら、端末のベルを鳴らす。
- **関係**：hook だけで、ほかのプラグインに依存しない。中の構成と、どの場面で何回鳴らすかは
  [bell の文書](../../plugins/bell/docs/README.md)。
