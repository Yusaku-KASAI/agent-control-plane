# プラグイン

platform の各プラグインの責務と関係。

## 構成

各プラグインは `plugins/<名前>/` にあり、hook、スキル、コマンド（`bin/`）、スクリプトを持つ。プラグインの間の
関係の全体は [overview.md](../overview.md) にある。

## 構成要素

### `roles`

- **責務**：共通のロール（Claude Code のカスタムエージェント）を定義する。developer（変更を実装する）と guide（質問に
  答え、変更しない）。
- **置くもの / 置かないもの**：ロールの振る舞いと、そのロールが使うモデル（[ADR 0003](../adr/0003-roles-choose-their-model.md)）。
  ターゲット固有の事実や、ハーネスの事実（環境変数、ブランチの決め方）は置かない。
- **関係**：harness が伝える動き方とハーネスの契約、conventions の決まりと観点を前提にする。github と slack のスキルを使う。
- **当てはめた原則・パターン**：[関心の分離](../../plugins/principles/docs/reference/principles.md#soc)：振る舞いだけを持ち、事実は契約とターゲットの CLAUDE.md に任せる。
  [段階的な開示](../../plugins/principles/docs/reference/principles.md#progressive-disclosure)：ロールの定義は常に読み込まれるので、観点や手順の中身はスキルを参照する。

### `principles`

- **責務**：どこでも通用する設計の原則とパターンを、スキルと文書で届ける。
- **関係**：ほかのどのプラグインも前提にしない。conventions と、各リポジトリの構成要素の説明から指される。詳しくは
  [principles の文書](../../plugins/principles/docs/README.md)。
- **当てはめた原則・パターン**：[依存の向き](../../plugins/principles/docs/reference/principles.md#dependency-rule)：いちばん内側。

### `conventions`

- **責務**：このプロジェクト群の決まり（書き方、文書の体系と形式、作業の進め方、設計の観点）を届ける。いつも守らせる
  決まりはセッションとサブエージェントの始めに読み込ませ（[ADR 0004](../adr/0004-common-rules-via-session-start-hook.md)）、
  手順と観点はスキル（write-docs、design）にする。
- **関係**：principles を前提にする。詳しくは [conventions の文書](../../plugins/conventions/docs/README.md)。
- **当てはめた原則・パターン**：[共有の核](../../plugins/principles/docs/reference/patterns.md#shared-kernel)：principles とあわせて、
  すべての区切りが前提にする核。

### `harness`

- **責務**：エージェントに、どう起動されているか（control-plane 経由か devcontainer で直接か）と、ハーネスの契約と
  用語集の在りかを、セッションとサブエージェントの始めに伝える。詰まったときの切り分けのスキルを持つ。
- **置くもの / 置かないもの**：数行の案内と、切り分けの手順。契約の中身は [reference/harness.md](../reference/harness.md) に置く。
- **関係**：文書のパスは、プラグインの隣の `docs/` から決める。見つからなければ `/workspace/agent-platform/docs` を使う。
- **当てはめた原則・パターン**：[使う側に届く契約](../../plugins/principles/docs/reference/patterns.md#published-language)：control-plane のリポジトリを読めないエージェントにも、
  契約を届ける。[段階的な開示](../../plugins/principles/docs/reference/principles.md#progressive-disclosure)：始めに渡すのは行き先だけ。

### `github`

- **責務**：PR の本文を今のブランチに合わせ続ける。`prctl` コマンド、push の直後とターンの終わりの hook、GitHub での
  作業のスキル（[ADR 0002](../adr/0002-pr-body-tracks-head.md)）。
- **関係**：`gh` の認証を使う（エージェントには control-plane が `GH_CONFIG_DIR` で渡す）。
- **当てはめた原則・パターン**：[仕組みで守る](../../plugins/principles/docs/reference/principles.md#mechanisms)：本文が古いことを hook が見つけ、書き換えを促す。

### `slack`

- **責務**：Slack を読み書きする `slackctl` コマンドと、その使い方のスキル。
- **関係**：`SLACK_BOT_TOKEN_FILE`（呼ばれるたびに読む）か `SLACK_BOT_TOKEN` を使い、なければ投稿だけを
  `SLACK_WEBHOOK_URL` で行う。

<a id="bell"></a>

### `bell`

- **責務**：対話で使っている Claude Code が止まったら、端末のベルを鳴らす。
- **関係**：hook だけで、ほかのプラグインに依存しない。

対話で使っている Claude Code が止まったら、hook が端末のベルを鳴らす。聞き逃さないよう、どの場面でも2回以上鳴らす。
すぐに人の操作が要る場面（操作すれば再開できる、または直す必要がある）は短い間隔で2回、ターンを終えて結果を
見ればよい場面は3回にして、聞き分けられるようにする。

| 止まり方 | hook | ベル |
| --- | --- | --- |
| ターンの終わり | `Stop` | 3回、0.8秒おき |
| API のエラーでの終わり（利用上限、認証の失敗など） | `StopFailure` | 2回、0.4秒おき |
| 権限の確認 | `PermissionRequest` | 2回、0.4秒おき |
| 質問、プランの承認 | `PreToolUse`（`AskUserQuestion`、`ExitPlanMode`） | 2回、0.4秒おき |
| 入力待ちが続く、MCP やエージェントからの入力の依頼 | `Notification`（`idle_prompt`、`elicitation_dialog`、`elicitation_url_dialog`、`agent_needs_input`） | 2回、0.4秒おき |

- hook は自分の端末を持たないので、祖先のプロセスのうち端末を持つもの（対話で動いている Claude Code）の端末に、
  間隔を空けてベルを書く。書くのは裏で行い、hook は待たずに戻る。tmux の中でも外でも同じ。
- `-p` で動くエージェントには端末を持つ祖先がないので、鳴らない。
- 人が Esc で止めたときは、人がその場にいるので鳴らさない（`Stop` も動かない）。プロセスが落ちたときは hook が動かない。
