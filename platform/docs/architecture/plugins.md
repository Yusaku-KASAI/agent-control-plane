# プラグイン

platform の各プラグインの責務と、プラグインの間の関係。各プラグインの中の構成と構成要素は、そのプラグインの文書に
ある。

## 構成

```
plugins/
  harness/
  roles/     ──前提にする──▶ harness、github
  github/
  slack/
  bell/
```

- 各プラグインは `plugins/<名前>/` にあり、hook、スキル、コマンド（`bin/`）、スクリプト、カスタムエージェント
  （`agents/`）、CI のアクション（`actions/`）のうち要るものを持つ。文書の根はそれぞれの `docs/`。
- platform の外では、どのプラグインも principles、conventions と契約（エージェントの用語集、ハーネスの契約、
  ターゲットの契約）を前提にしてよい。各プラグインが何を前提にするかは、下の各構成要素の「関係」にある。

## 構成要素

### `harness`

- **責務**：エージェントに、どう動かされているかと、ハーネスの契約と platform の文書の一覧の在りかを伝え、詰まったときの
  切り分けを助ける。control-plane 経由のときは、サブエージェントをフォアグラウンドで動かす（[ADR 0008](../adr/0008-subagents-in-foreground-under-control-plane.md)）。
- **置くもの / 置かないもの**：短い案内と、切り分けの手順。契約の中身は[ハーネスの契約](../reference/harness.md)に置く。
- **関係**：ほかのプラグインを前提にしない。roles が前提にする。中の構成は [harness の文書](../../plugins/harness/docs/README.md "在りか")。
- **当てはめた原則・パターン**：[使う側に届く契約](../../plugins/principles/docs/reference/patterns.md#published-language)：control-plane のリポジトリを読めない
  エージェントにも、契約を届ける。

### `roles`

- **責務**：共通のロール（Claude Code のカスタムエージェント）を定義する。
- **置くもの / 置かないもの**：受け持つ手順（書ける範囲と、振り分けの目的の手順ごとの扱い。考え方は conventions の
  [目的の手順と共通の手順](../../plugins/conventions/docs/architecture/documentation.md#procedures)、定義の形は
  [形式：役割ごとの指示](../../plugins/conventions/docs/reference/formats.md#role)）と、ロールに固有の振る舞いと、そのロールが使う決まりの選び出し（conventions の定義を要約か案内で載せる）、使うモデル（[ADR 0003](../adr/0003-roles-choose-their-model.md)）。
  ターゲット固有の事実や、ハーネスの事実は置かない。
- **関係**：harness が伝える動き方とハーネスの契約、conventions の決まりと観点、github のスキルを前提にする。
  中の構成は [roles の文書](../../plugins/roles/docs/README.md "在りか")。
- **当てはめた原則・パターン**：[関心の分離](../../plugins/principles/docs/reference/principles.md#soc)：振る舞いだけを持ち、事実は契約とターゲットの
  CLAUDE.md に任せる。

### `github`

- **責務**：PR の本文と差分のレビューを今のブランチに追いつかせ続け、GitHub での作業（PR、レビュー、CI）の手順を届ける
  （[ADR 0002](../adr/0002-pr-body-tracks-head.md)、[ADR 0005](../adr/0005-self-review-tracks-head.md)、
  [ADR 0006](../adr/0006-ci-failures-stop-the-turn.md)）。ターゲットの既定のブランチの守りが外れていないことを確かめる
  コマンドと、それを CI で回す複合アクションを配る（[target-template の ADR 0016](../../../template/docs/adr/0016-protection-checked-in-ci.md)）。
  開いている Issue とロードマップのずれのうち判断の要らないものを挙げるコマンドも配る（[ADR 0023](../adr/0023-check-issues-on-demand.md)）。
- **関係**：ハーネスの契約で渡される `gh` の認証を前提にする（[ハーネスの契約](../reference/harness.md)）。roles が
  前提にする。中の構成は [github の文書](../../plugins/github/docs/README.md "在りか")。
- **当てはめた原則・パターン**：[仕組みで守る](../../plugins/principles/docs/reference/principles.md#mechanisms)：本文が古いことを hook が見つけ、
  書き換えを促す。

### `slack`

- **責務**：Slack のメッセージとスレッドを読み、投稿する。control-plane 経由では、ターンの最後の回答に Slack で崩れる形が
  ないかを、ターンの終わりの hook で確かめる。
- **関係**：ハーネスの契約で渡される認証情報と動き方を前提にし、認証情報の組み合わせでできることが決まる
  （[ハーネスの契約](../reference/harness.md)）。中の構成は [slack の文書](../../plugins/slack/docs/README.md "在りか")。

### `bell`

- **責務**：対話で使っている Claude Code が止まったら、端末のベルを鳴らす。
- **関係**：hook だけで、ほかのプラグインを前提にしない。中の構成と、どの場面で何回鳴らすかは
  [bell の文書](../../plugins/bell/docs/README.md "在りか")。
