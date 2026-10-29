# 依頼と知らせ

依頼が control-plane にどう入り、結果と知らせがどう依頼者に戻るか。使う人から見た依頼の仕方は
[依頼の仕方](../../reference/usage.md "在りか")に、エージェントから見た依頼と回答は
[ハーネスの契約](../../../platform/docs/reference/harness.md "在りか")にある。プロセスの中の構成要素は
[control-plane の構成](components.md "在りか")にある。

## 構成

依頼の入口は二つあり、どちらも `Dispatcher.Execute` に依頼を渡して、それに答えるターンが終わるまで待つ。

```
Slack のメンション ─▶ slackbot.Bot ─▶ HandleMessage ─▶ Dispatcher ─▶ セッションのワーカー
                         ▲               │ 返事、リアクション
                         └───────────────┘
Slack のリアクション ─▶ slackbot.Bot ─▶ HandleMessage ─▶ Dispatcher（停止）／ SessionCleaner（今すぐの片付け）
acp run / stop ─▶ httpapi ─▶ Dispatcher（応答の本文で結果を返す）

依頼を待つ人のいない知らせ：
  ワーカー ─▶ AgentAnswers ─▶ sessionObserver ─▶ slackbot.Bot（状態メッセージ、自分で始めたターンの回答、中断）
  SessionCleaner ─▶ sessionObserver ─▶ slackbot.Bot（片付け）
```

- **依頼への返事**は、依頼を受けた入口が返す。Slack では `HandleMessage` が結果に合わせてスレッドに返信し、
  依頼のメッセージにリアクションを付ける。HTTP API では応答の本文になる。
- **依頼を待つ人のいない知らせ**は、`serve.go` の `sessionObserver` が受けて、知らせる場所があれば Bot に渡す
  （[sessionObserver](#observer)）。
- 依頼をどの順にエージェントに送り、停止で何を止めるかは、セッションのワーカーが決める
  （[セッションのワーカー](sessions.md#worker "在りか")）。

依頼のメッセージに付くリアクション。依頼がどこまで進んだかを、付けたものを外さずに足していく。どの段階のものが
付いているか（受け付けた → 取りかかった → 終わった）で今の状態が分かる（[ADR 0038](../../adr/app/0038-reactions-tell-how-requests-were-taken.md)）：

| リアクション | 付くとき |
| --- | --- |
| `:inbox_tray:` | 依頼を受け付けた。エージェントが取り込むまではこれだけが付く |
| `:eyes:` | エージェントが依頼を取り込み、取りかかった |
| `:zap:` | 流し込んだ依頼を、エージェントが実行中のそのターンに取り込んだ。本文を `:zap:` で始めても、実行中のターンがなかったときと、エージェントが次のターンに回したときは `:eyes:` になる。依頼のターンが隠すエージェントのターン（[セッションのワーカー](sessions.md#worker "在りか")）は、依頼のターンの一部として数える |
| `:white_check_mark:` | 答えた。回答を返した、または同じターンのほかの依頼への回答がこの依頼の答えも兼ねている |
| `:warning:` | エラー（使用上限を含む）や失敗で終わった |
| `:no_entry_sign:` | 依頼が停止で取り消された、ターンが中断された、再起動で中断された |

スレッドに投稿されるもの：

| 投稿 | 投稿するもの | きっかけ |
| --- | --- | --- |
| ターンの回答 | `HandleMessage` | 依頼に答えるターンが終わった |
| 状態メッセージ | `slackbot.Bot` | 依頼に答えるターンが始まった。エージェントが自分で始めたターンは、最後の状態メッセージを書き換えて使う |
| 自分で始めたターンの回答 | `AgentAnswers`（`slackbot.Bot` が書く） | エージェントが自分で始めたターンが終わった。いつ投稿するか（処理が残る間は持っておく、エラーのとき、使用上限の知らせ）は[自分で始めたターンの回答と中断の知らせ](#agent-answer-rules)にある |
| 失敗や取り消しの文面 | `HandleMessage` | 依頼者が許可されていない、ターンが失敗した、エージェントがエラー（使用上限を含む）でターンを終えた、停止や再起動で取り消された |
| 中断の知らせ | `AgentAnswers`（`slackbot.Bot` が書く） | 待っている依頼がないのに進めていた処理を、control-plane が止まるときや起動したときに止めた（[自分で始めたターンの回答と中断の知らせ](#agent-answer-rules)） |
| 片付けの知らせ | `SessionCleaner` | 保持期間を過ぎて判定したか、人がスレッドの最初のメッセージへのリアクションで頼んだ。何を知らせるかは[片付け](cleanup.md#cleaner "在りか")にある |

<a id="agent-answer-rules"></a>

### 自分で始めたターンの回答と中断の知らせ

依頼を待つ人のいないターン（エージェントが自分で始めたターン）の回答と中断の知らせを、何をいつスレッドに出すか。
決めるのは [`AgentAnswers`](#agent-answers)、投稿するのは `slackbot.Bot`。エージェントから見た規則は
[ハーネスの契約](../../../platform/docs/reference/harness.md#requests "在りか")にあり、ここではそれをどう実装するかを書く。
バックグラウンドの処理が残っている間に終わったターンの回答は持っておき、出来事ごとに次のように扱う。上の行から順に、
最初に当たる行で扱う。「終わった」は中断されて終わったものも含む。「依頼の入ったターン」は依頼を取り込んだターン
（自分で始めたターンに流し込んだ依頼が入ったものを含む）。自分で始めたターンが依頼の入ったターンとして終わったときは
2 行目に当たるので、それより下の行の「自分で始めたターンが…終わった」は依頼の入らなかったもの。

| 出来事 | 持っておいた回答の扱い |
| --- | --- |
| ターン（依頼の入ったターンも）が、エラーかプロセスの失敗で終わった | 投稿する。自分で始めたターン（依頼の入らなかったもの）がエラーで終わったときは、エラーの知らせの後ろに置いて一つのメッセージにする |
| 依頼に答えるターンが始まったか、依頼の入ったターンが終わった | 捨てる |
| 自分で始めたターンが、中断されて回答を出したか、処理を残さずに回答を出して終わった | 捨て、そのターンの回答を投稿する |
| 自分で始めたターンが、処理を残して回答を出して終わった | その回答を、前に持っていたものに代えて持つ |
| ターンのないまま処理が尽きたか、自分で始めたターンが回答を出さずに処理を残さずに終わった | 15 秒待ち、その間にターンが始まらなければ投稿する |
| 自分で始めたターンが、回答を出さずに処理を残して終わった | 持ち続ける |
| 自分で始めたターンが始まった | 持ち続ける（15 秒の待ちは止める） |
| 15 秒の待ちの間に control-plane が止まる | 待たずに投稿する（`Flush`。`Dispatcher.Close` が最後の出来事のあとに呼ぶ） |
| 中断の知らせを出す | 捨てる |

自分で始めたターンがエラー（使用上限を含む）で終わったら、バックグラウンドの処理が残っていても、エラーの知らせを
すぐ投稿する（[ADR 0029](../../adr/app/0029-turns-ending-on-errors.md)）。使用上限の知らせは、上限ではない結果で
終わるターンがあるまで、続けて出さない。

## 構成要素

### `slackbot.Bot`（`bot.go`）

- **責務**：エージェントの Slack アプリとしてメンションを依頼に直して `HandleMessage` に渡し、返事をスレッドに
  書く。
- **置くもの / 置かないもの**：
  - メンションのスレッド（スレッドの外のメンションなら、そのメッセージ自身）を依頼のスレッドにする。スレッドからセッション ID が
    決まる（[セッション ID](sessions.md#session-id "在りか")）。
  - プロンプトには本文を入れず、依頼したユーザー、依頼のメッセージとスレッドの場所と、最後の回答は control-plane が
    投稿するので同じ回答をそのスレッドに投稿しないこと（ほかの投稿は頼まれればしてよい。[platform の ADR 0027](../../../platform/docs/adr/0027-agents-post-to-slack-when-asked.md)）と、
    回答を slack スキルの書き方で書くこと（回答は Slack で表示されるため）だけを書く。本文は、
    `:zap:` で始まるか（流し込み）を見るためだけに使う（[ADR 0020](../../adr/app/0020-slack-requests-by-reference.md)）。
  - Bot が書いたメッセージは依頼にしない。Slack が送り直した同じメッセージ（チャンネルと ts が同じ）は、
    1 時間のあいだ一度だけ扱い、送り直されたものには何も返さない（[ADR 0038](../../adr/app/0038-reactions-tell-how-requests-were-taken.md)）。
  - 依頼のメッセージへの `:no_entry_sign:` を、その依頼の停止として `HandleMessage.StopRequest` に渡す。依頼の
    メッセージがどのスレッドのものかはメモリに 24 時間覚え、覚えていないメッセージ（再起動の前の依頼、依頼でない
    メッセージ）へのリアクションは何もしない。依頼できる人の指示だけが効く
    （[ADR 0039](../../adr/app/0039-stop-by-reacting-to-request-messages.md)）。
  - スレッドの最初のメッセージ（ルート）への `:wastebasket:` と `:fire:` を、そのスレッドのセッションを今すぐ片付ける指示
    （`:fire:` は失われるものを承知で）として `HandleMessage.CleanSession` に渡す。最初のメッセージの ts がスレッドの
    ts なので、覚えていなくてもセッションが決まり、再起動のあとでも効く。依頼できる人の指示だけが効く。止まるときは
    取り消す（[ADR 0046](../../adr/app/0046-clean-a-session-now-by-reacting.md)）。
  - 依頼ごとにゴルーチンで `HandleMessage` を動かす。サーバーが止まるときも依頼の処理は取り消さず、
    `Dispatcher` が閉じて依頼が終わるのを待つ。
  - 返事と状態メッセージは、Slack の markdown ブロックが標準の Markdown と違って読む形のうち、意味を変えずに直せる
    ものを直してから送る。裸の URL は `<…>` で囲み、文字の行のすぐ下の表には前に空行を足す。コードブロックと
    インラインコードの中と、リンクの記法の中は変えない。コードブロックは閉じの行まで続き、閉じの行がないものだけ、
    引用が終わるか、リストの項目の兄弟の印の行で閉じる（CommonMark に倣う）。直せない形（太字の内側の ASCII の記号など）は、エージェントの書き方に任せる
    （[Slack の markdown ブロックが読む形](../../../platform/plugins/slack/docs/reference/markdown.md)、
    [ADR 0050](../../adr/app/0050-fix-slack-markdown-forms-before-posting.md)）。
  - 直す場所は、行ごとの手書きの走査で見つける。試験は、slack プラグインの例の一覧
    （`platform/plugins/slack/tests/markdown-cases.json`）の文を直し、直した文を確かめ、その文でプラグインの
    `check-markdown` が裸の URL とすぐ上が文字の行の表を知らせないことも確かめる（[ADR 0051](../../adr/app/0051-own-markdown-scan-checked-by-shared-cases.md)）。
  - 返事は Slack の markdown ブロックの上限（12,000 文字）に収まるよう、11,000 文字で切り詰めて省いたことを書き添える。
- **関係**：`ChatResponder` を満たす。Bot のトークンは毎回 `slackauth.Source` から受け取る。
- **当てはめた原則・パターン**：[関心の分離](../../../platform/plugins/principles/docs/reference/principles.md#soc)：Slack の受け方と書き方
  だけを持ち、依頼の扱いは `HandleMessage` に任せる。Slack に固有のこと（記号、ブロック、文字数の上限）はここに
  閉じる（[ADR 0016](../../adr/app/0016-chat-entry-point.md)）。

<a id="status"></a>

### `slackbot` の状態メッセージ（`status.go`）

- **責務**：依頼に答えるターンごとにスレッドへ状態メッセージを出し、ターンが終わるまで書き換える
  （[ADR 0019](../../adr/app/0019-chat-controls-and-status.md)）。
  エージェントが自分で始めたターンは、そのセッションの最後の状態メッセージを書き換えて使う
  （[ADR 0028](../../adr/app/0028-quiet-agent-started-turns.md)）。自分で始めたターンの回答と中断の知らせは、
  [`AgentAnswers`](#agent-answers) が決めた文面（`SessionNotice`）をそのまま投稿する。
- **置くもの / 置かないもの**：
  - **見せるもの**：状態（実行中、完了、中断、失敗、使用上限で停止）、経過時間、実行中はエージェントの
    直近 10 件の発言と今の作業、終わったあとは費用の見積もり、待っている依頼とバックグラウンドの処理の数、
    セッション ID、止められる間は止め方（依頼のメッセージに `:no_entry_sign:` を付ける）。費用の見積もりは、その
    状態メッセージが追ったターン（引き継いだターンと、依頼のターンが隠したターンを含む）の和と、セッションの合計
    （[ADR 0037](../../adr/app/0037-usage-in-status-messages.md)）。
    どれか一つのターンの費用が分からなければ、和は「不明」と出す。`result` の出なかったターンの費用は、次の依頼の
    和に入ることがある。値は worker が `SessionEvent.Spent` で渡す
    （[使用量の記録](sessions.md#usage "在りか")）。
  - **締めの促し**：終わったあと、その状態メッセージが追ったターンのうち最後に分かったコンテキストの大きさが
    `sessions.suggest_closing_at`（[acp.example.yaml](../../../acp.example.yaml "在りか")）以上なら、費用の見積もりの次に、締めて次の作業を新しい
    スレッドで依頼するよう促す 1 行を出す。知らせるだけで、自動では締めない（[ADR 0049](../../adr/app/0049-suggest-closing-when-the-context-grows.md)、[0052](../../adr/app/0052-keep-the-closing-threshold-at-400k.md)）。
    コンテキストの大きさは worker が `TurnResult.ContextTokens` で渡す。
  - **書き換え**：エージェントの発言は状態メッセージが持ち、引き継いでも残る
    （[ADR 0034](../../adr/app/0034-agent-remarks-in-status-messages.md)）。書き換えは 5 秒ごとと、変化から 1 秒
    まとめたあと。引き継いだ状態メッセージの経過時間は、それを出した依頼のターンの始まりから数える。最後の状態
    メッセージを知らないときは新しく出す。
- **関係**：`sessionObserver` から `SessionEvent` を受ける。
- **当てはめた原則・パターン**：[高凝集・疎結合](../../../platform/plugins/principles/docs/reference/principles.md#cohesion)：ワーカーは
  状態の値だけを伝え、どう見せるかと Slack の API の回数の抑え方はここで決める。

### `usecase.HandleMessage`（`handle_message.go`）

- **責務**：チャットから届いた依頼を、受信方式に関係なく、依頼者の確認、受付の合図、ターンの実行、返事の順に扱う。
  依頼のメッセージへの停止の指示を、依頼できる人か確かめてから、その依頼の停止として `Dispatcher.StopRequest` に渡す
  （[ADR 0039](../../adr/app/0039-stop-by-reacting-to-request-messages.md)）。スレッドの最初のメッセージへの片付けの
  指示を、依頼できる人か確かめてから、そのスレッドのセッションの今すぐの片付けとして `SessionCleaner.CleanNow` に渡す
  （`CleanSession`、[ADR 0046](../../adr/app/0046-clean-a-session-now-by-reacting.md)）。
- **置くもの / 置かないもの**：結果ごとの合図と文面。
  - 許可されていない依頼者には、受け付けないと返す。依頼できるのは、エージェントごとの `allowed_users` に
    載っている人だけ。
  - 受け付けたら `:inbox_tray:` を付け、エージェントが依頼を取り込んだと `Dispatcher` から知らされたら、取り込まれ方に
    合わせて `:eyes:` か `:zap:` を足す。終わったら、返事を返してから、終わり方の合図を足す。
  - ターンの回答を返信する。回答が別の依頼への返信と同じなら返信せず、答えたなら `:white_check_mark:` を、そのターンが
    エラーで終わったなら `:warning:` を付ける。
  - 中断されたターンは、回答があればそれを、なければ中断したことを返す。
  - エージェントがエラーで終えたターンは、エラーであることとエージェントの文面を引用して返す。使用上限で止まった
    ときは、そう書き、上限が戻ったあとにもう一度依頼すれば続きから進むことを添える
    （[ADR 0029](../../adr/app/0029-turns-ending-on-errors.md)）。
  - 停止と再起動で取り消された依頼には、そう返す。
  - 失敗には、セッションが使用中、ワーカーが受け取れていない依頼が多すぎる、内容が空、それ以外の失敗
    （セッション ID とエラー）を書き分けて返す。
- **関係**：スレッドからセッション ID を決め、なければ作るよう `Dispatcher` に頼む。
- **当てはめた原則・パターン**：[層に分けた構成](../../../platform/plugins/principles/docs/reference/patterns.md#layered-architecture)：返事の手段は
  `ChatResponder` で、Slack 以外の受け方も同じユースケースを使える（[ADR 0016](../../adr/app/0016-chat-entry-point.md)）。
  [拒否を既定にする](../../../platform/plugins/principles/docs/reference/principles.md#fail-safe-defaults)：`allowed_users` に載っている人の依頼だけを受け、ほかはすべて断る。

<a id="agent-answers"></a>

### `usecase.AgentAnswers`（`agent_answers.go`）

- **責務**：依頼を待つ人のいないターン（エージェントが自分で始めたターン）の回答と、中断の知らせのうち、何をいつスレッドに
  出すかを決める（[ADR 0028](../../adr/app/0028-quiet-agent-started-turns.md)、[0029](../../adr/app/0029-turns-ending-on-errors.md)、[0055](../../adr/app/0055-agent-answers-decided-in-usecase.md)）。
- **置くもの / 置かないもの**：ワーカーから受けた出来事をそのまま次（`sessionObserver`）に渡し、そのあとで、出すと決めた
  文面を `SessionNotice` として渡す。セッションごとに、持っておいた回答と、使用上限を知らせたかを持つ。Slack の
  書き方と投稿は置かない。規則は[自分で始めたターンの回答と中断の知らせ](#agent-answer-rules)にある。
  包む先へは、ワーカーのほか、待ちが過ぎたときのタイマーと、止まるときの `Flush` からも渡す（順はロックで保つ）。
- **関係**：`Dispatcher` には `SessionObserver` として渡され、`sessionObserver` を包む（`serve.go` が組み立てる）。
  使用上限の知らせとエラーの知らせの文面は `HandleMessage` と同じ `TurnErrorText` を使う。
- **当てはめた原則・パターン**：[事実と判定を分ける](../../../platform/plugins/principles/docs/reference/patterns.md#facts-and-judgement)：Bot は出来事を見せることと
  決まった文面を投稿することだけを持ち、出すか、持つか、捨てるかの判定はここに置く。
  [状態を軸で洗い出す](../../../platform/plugins/principles/docs/reference/patterns.md#state-axes)：上の表の行ごとに試験（`agent_answers_test.go`）を書く。

<a id="shutdown"></a>

### 停止と再起動の知らせ（`Dispatcher.Close`、`Dispatcher.Recover`）

- **責務**：control-plane が止まるときと起動したときに、動いていた処理を中断し、スレッドに知らせる
  （[ADR 0024](../../adr/app/0024-restart-abandons-running-work.md)）。
- **置くもの / 置かないもの**：
  - 止まるとき：ワーカーごとに実行中のターンを中断してからプロセスを閉じる。待っている依頼には再起動で中断した
    ことを返し、待っている依頼がないのに進めていた処理（自分で始めたターン、バックグラウンドの処理）があれば、
    スレッドに同じことを知らせる。知らせ終えるまで compose が待つ猶予は[コンテナ](containers.md#service "在りか")にある。
    止まり始めたあとに届いた依頼は受け付けない。
  - 起動するとき：依頼を受け付ける前に、前の実行が残したエージェントのコンテナを runner に消させ
    （[コンテナ](containers.md#runner "在りか")の `RemoveLeftovers`）、そのセッションのスレッドに、再起動で止めたことを知らせる。
- **関係**：知らせは `SessionAbandoned` として [`AgentAnswers`](#agent-answers) に伝わり、そこで決めた文面が `sessionObserver` に伝わる。

<a id="observer"></a>

### `sessionObserver`（`cmd/acp/serve.go`）

- **責務**：セッションの出来事と知らせを、セッションのエージェントの Bot に渡す。
- **関係**：`SessionObserver` と `SessionNotifier` を満たす。Slack のスレッドから始まったセッションだけを Bot に
  渡し、それ以外は自分で始めたターンの回答をログに書くだけにする。
- **当てはめた原則・パターン**：[層に分けた構成](../../../platform/plugins/principles/docs/reference/patterns.md#layered-architecture)：ユースケースは
  知らせる先を知らず、組み立ての場所で入口と結ぶ。

<a id="http-api"></a>

### `httpapi` のサーバー（`server.go`）

- **責務**：HTTP API で依頼と停止を受け、ターンの結果を応答として返す。
- **置くもの / 置かないもの**：ルーティング、トークンの確認、JSON の形、エラーと HTTP の状態の対応。

  | エンドポイント | すること |
  | --- | --- |
  | `GET /healthz` | 動いていることを返す |
  | `POST /v1/turns` | 依頼する。`session_id` を省くとランダムな ID の新しいセッションを始め、付けるとそのセッションの続きにする（なければエラー）。ターンが終わるまで応答を返さない |
  | `POST /v1/sessions/{id}/stop` | セッションを停止する |

  - `/v1/` は `Authorization: Bearer <ACP_API_TOKEN>` を求める。依頼者の確認はこのトークンだけで、
    `allowed_users` は見ない。トークンを持つ人を依頼者として扱い、誰が依頼したかは区別しない
    （[ADR 0014](../../adr/app/0014-control-plane-as-a-server.md)）。
  - エージェントがエラーで終えたターンは、ターンの応答（200）に `error` を、使用上限で止まったときは `usage_limited` も
    立てて返す。
  - エラーは HTTP の状態と `code`（`session_busy`、`inbox_full`、`unavailable` など）で返し、セッションが
    あればその ID と作業領域のパスを添える。
  - スレッドを持たないセッションなので、状態メッセージも、依頼を待つ人のいない知らせも届かない（[sessionObserver](#observer)）。
- **関係**：`Dispatcher` を、自分で定めた `TurnRunner` として呼ぶ。クライアントは `httpapi.Client` で、`acp run` と
  `acp stop` が使う（[依頼の仕方](../../reference/usage.md "在りか")）。
- **当てはめた原則・パターン**：[スキーマを先に](../../../platform/plugins/principles/docs/reference/principles.md#schema-first)：要求、応答、
  エラーの形を `types.go` に定め、サーバーとクライアントがそれを共有する。
  [拒否を既定にする](../../../platform/plugins/principles/docs/reference/principles.md#fail-safe-defaults)：`ACP_API_TOKEN` がなければ起動せず、`/healthz` のほかは、トークンが
  合わない要求をすべて断る。
  [外から入る値を信用しない](../../../platform/plugins/principles/docs/reference/principles.md#untrusted-input)：要求の本文は知らないフィールドを拒み、大きさを限って読む。
  セッション ID は決まった形（`domain.ParseSessionID`）かを確かめてから使う。
