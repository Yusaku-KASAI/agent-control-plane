# platform の全体像

platform は、ターゲットをまたいで使うエージェントの能力（ロール、スキル、hook）を、Claude Code のプラグインとして
まとめたもの。プラグインはマーケットプレイス `acp-platform` に入れて配る（[ADR 0001](adr/0001-platform-as-plugin-marketplace.md)）。

platform は principles、conventions と契約（[エージェントの用語集](reference/glossary.md)、[ハーネスの契約](reference/harness.md)、
[ターゲットの契約](reference/target.md)）を前提にし、エージェントを動かす側（control-plane と devcontainer）とターゲットとは、
契約だけでつながる。区切りの分け方と依存の向きの決まりは、conventions の
[文書の体系](../plugins/conventions/docs/architecture/documentation.md)にある。

## 構成

```
platform/
  .claude-plugin/marketplace.json   マーケットプレイス acp-platform
  plugins/
    harness/ roles/ github/ …       エージェントの能力（platform の中の項目。下の構成要素）
    principles/                     どこでも通用する原則とパターン（別の区切り）
    conventions/                    このプロジェクト群の決まり（別の区切り）
  docs/                             platform の文書（README.md が一覧）と、契約の文書の置き場所
```

- `platform/` というディレクトリは、platform の置き場所であると同時に、マーケットプレイスとしてほかの区切りも運ぶ
  入れ物を兼ねる（何を運ぶかは[用語集](reference/glossary.md "在りか")のマーケットプレイス）。運ばれるほかの区切りは
  platform に属さない。
- `docs/` には platform の文書を置く。置き場所として、契約という項目に属するエージェントの用語集、ハーネスの契約、
  ターゲットの契約もここに置く。
- 各プラグインは `plugins/<名前>/` にあり、hook、スキル、コマンド（`bin/`）、スクリプト、カスタムエージェント
  （`agents/`）、CI のアクション（`actions/`）のうち要るものを持つ。文書の根はそれぞれの `docs/` で、中の構成はそこに書く。
- プラグインには、どのターゲットでも通用する内容だけを書く。プラグインの形は `claude plugin validate` で、`bin/` と
  `scripts/` のシェルスクリプトは `shellcheck` で確かめる。
- どのプラグインも、platform の外では principles、conventions と契約を前提にしてよい。プラグインの間の前提は
  [地図](#map-platform)にだけ書く。

## 届き方

- ターゲットは `.claude/settings.json` で `acp-platform` を宣言し、使うプラグインを有効にする。
- control-plane から起動するエージェントには、control-plane が宣言を読んで `--plugin-dir` で読み込ませる。プラグインに
  渡す認証情報や環境は、[ハーネスの契約](reference/harness.md)にある。
- 人が対話で使うときは、[対話で使う準備](guides/use-interactively.md)のとおりに入れる。

<a id="components"></a>

## 構成要素

### `harness`

- **責務**：エージェントに、どう動かされているかと、ハーネスの契約と platform の文書の一覧の在りかを伝え、契約の制約を
  守らせ、詰まったときの切り分けを助ける。
- **置くもの / 置かないもの**：短い案内、hook、切り分けの手順。control-plane 経由のときの hook は、サブエージェントを
  フォアグラウンドで動かし（[ADR 0008](adr/0008-subagents-in-foreground-under-control-plane.md)）、終わりの知らせが届かない
  バックグラウンドの処理と見張りを断る（[ADR 0015](adr/0015-refuse-let-go-processes-under-control-plane.md)）。あわせて、
  サブエージェントの間に作業領域が変われば知らせる（[ADR 0016](adr/0016-subagents-leave-the-work-area.md)）。契約の中身は
  [ハーネスの契約](reference/harness.md)に置く。
- **関係**：中の構成は [harness の文書](../plugins/harness/docs/README.md "在りか")。
- **当てはめた原則・パターン**：[使う側に届く契約](../plugins/principles/docs/reference/patterns.md#published-language)：control-plane のリポジトリを読めない
  エージェントにも、契約を届ける。[仕組みで守る](../plugins/principles/docs/reference/principles.md#mechanisms)：契約の制約を hook で守らせる。

### `roles`

- **責務**：共通のロール（Claude Code のカスタムエージェント）を定義する。
- **置くもの / 置かないもの**：ロールの振る舞いだけを置き、ターゲット固有の事実や、ハーネスの事実は置かない。
- **関係**：中の構成は [roles の文書](../plugins/roles/docs/README.md "在りか")。
- **当てはめた原則・パターン**：[関心の分離](../plugins/principles/docs/reference/principles.md#soc)：振る舞いだけを持ち、事実は契約とターゲットの
  CLAUDE.md に任せる。

### `github`

- **責務**：PR の本文と差分のレビューと CI を今のブランチに追いつかせ続け、GitHub での作業（PR、レビュー、CI）の手順を
  届ける。
- **置くもの / 置かないもの**：PR を扱うコマンドと hook とスキル（[ADR 0002](adr/0002-pr-body-tracks-head.md)、
  [ADR 0005](adr/0005-self-review-tracks-head.md)、[ADR 0006](adr/0006-ci-failures-stop-the-turn.md)）。ターゲットの既定のブランチの守りが
  外れていないことを確かめるコマンドと、それを CI で回す複合アクション（[target-template の ADR 0016](../../template/docs/adr/0016-protection-checked-in-ci.md)）。
  開いている Issue とロードマップのずれのうち判断の要らないものを挙げるコマンド（[ADR 0023](adr/0023-check-issues-on-demand.md)）。
- **関係**：ハーネスの契約で渡される `gh` の認証を使う。中の構成は [github の文書](../plugins/github/docs/README.md "在りか")。
- **当てはめた原則・パターン**：[仕組みで守る](../plugins/principles/docs/reference/principles.md#mechanisms)：本文が古いことを hook が見つけ、
  書き換えを促す。

### `slack`

- **責務**：Slack のメッセージとスレッドを読み、投稿する。control-plane 経由では、ターンの最後の回答に Slack で崩れる形が
  ないかを、ターンの終わりの hook で確かめる。
- **関係**：ハーネスの契約で渡される認証情報と動き方を使い、認証情報の組み合わせでできることが決まる。中の構成は
  [slack の文書](../plugins/slack/docs/README.md "在りか")。

### `bell`

- **責務**：対話で使っている Claude Code が止まったら、端末のベルを鳴らす。
- **関係**：hook だけを持つ。中の構成と、どの場面で何回鳴らすかは [bell の文書](../plugins/bell/docs/README.md "在りか")。

<a id="map-platform"></a>

## 地図

platform の中の項目と前提の向き（[形式](../plugins/conventions/docs/reference/formats.md#map)）。

| 項目 | 親 | 置き場所 | 前提にする |
| --- | --- | --- | --- |
| harness | platform | `../plugins/harness/` | |
| github | platform | `../plugins/github/` | |
| slack | platform | `../plugins/slack/` | |
| roles | platform | `../plugins/roles/` | harness, github |
| bell | platform | `../plugins/bell/` | |
