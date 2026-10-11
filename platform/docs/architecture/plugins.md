# プラグイン

platform の各プラグインの責務と関係。各プラグインは `plugins/<名前>/` にあり、hook、スキル、コマンド（`bin/`）、
スクリプトを持つ。プラグインの間の関係の全体は [overview.md](../overview.md) にある。

### `roles`

- **責務**：共通のロール（Claude Code のカスタムエージェント）を定義する。developer（変更を実装する）と guide（質問に
  答え、変更しない）。
- **置くもの / 置かないもの**：ロールの振る舞いと、そのロールが使うモデル（[ADR 0003](../adr/0003-roles-choose-their-model.md)）。
  ターゲット固有の事実や、ハーネスの事実（環境変数、ブランチの決め方）は置かない。
- **関係**：harness が伝える動き方とハーネスの契約、review の観点を前提にする。github と slack のスキルを使う。
- **当てはめた原則**：[関心の分離](../reference/principles.md#soc)：振る舞いだけを持ち、事実は契約とターゲットの CLAUDE.md に任せる。
  [段階的な開示](../reference/principles.md#progressive-disclosure)：ロールの定義は常に読み込まれるので、観点や手順の中身はスキルを参照する。

### `rules`

- **責務**：どのターゲットでも守る決まり（コメントとドキュメントの書き方）を、セッションとサブエージェントの始めに
  読み込ませる（[ADR 0004](../adr/0004-common-rules-via-session-start-hook.md)）。
- **置くもの / 置かないもの**：一つひとつの文の書き方のような、いつも守らせたい短い決まり。分け方や置き場所の詳細は
  [文書の体系](documentation.md)に置き、そこを指す。
- **当てはめた原則**：[仕組みで守る](../reference/principles.md#mechanisms)：常に読み込む決まりの段。
  [段階的な開示](../reference/principles.md#progressive-disclosure)：短く保つ。

### `harness`

- **責務**：エージェントに、どう起動されているか（control-plane 経由か devcontainer で直接か）と、ハーネスの契約と
  用語集の在りかを、セッションとサブエージェントの始めに伝える。詰まったときの切り分けのスキルを持つ。
- **置くもの / 置かないもの**：数行の案内と、切り分けの手順。契約の中身は [reference/harness.md](../reference/harness.md) に置く。
- **関係**：文書のパスは、プラグインの隣の `docs/` から決める。見つからなければ `/workspace/agent-platform/docs` を使う。
- **当てはめた原則**：[境界づけられた文脈](../reference/principles.md#bounded-context)：control-plane のリポジトリを読めないエージェントにも、
  契約を届ける。[段階的な開示](../reference/principles.md#progressive-disclosure)：始めに渡すのは行き先だけ。

### `review`

- **責務**：設計とレビューの観点をスキルとして持つ。今は design（状態の軸、事実と判定の分離、失われうるもの、手順の
  突き合わせ、大きさ）。
- **置くもの / 置かないもの**：観点の定義。いつ使うかはロールが決める。
- **関係**：developer は作るときに当てはめ、PR の前のレビューでは抜けを探す観点として使う。
- **当てはめた原則**：[SSOT](../reference/principles.md#ssot)：観点の定義は一か所に置き、ロールとレビューから参照する。

### `github`

- **責務**：PR の本文を今のブランチに合わせ続ける。`prctl` コマンド、push の直後とターンの終わりの hook、GitHub での
  作業のスキル（[ADR 0002](../adr/0002-pr-body-tracks-head.md)）。
- **関係**：`gh` の認証を使う（エージェントには control-plane が `GH_CONFIG_DIR` で渡す）。
- **当てはめた原則**：[仕組みで守る](../reference/principles.md#mechanisms)：本文が古いことを hook が見つけ、書き換えを促す。

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
