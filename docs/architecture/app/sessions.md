# セッション

セッションが作られてから片付けられるまでと、セッションが持つもの。セッションと作業領域の意味は
[用語集](../../../platform/docs/reference/glossary.md)のとおり。エージェントから見たセッション（渡されるもの、
コンテナの寿命）は[ハーネスの契約](../../../platform/docs/reference/harness.md "在りか")にある。作業領域とブランチの
仕組みは[作業領域とブランチ](workspace.md "在りか")、コンテナは[コンテナ](containers.md "在りか")、認証情報は[認証情報](credentials.md "在りか")、
片付けは[片付け](cleanup.md "在りか")にある。

## 構成

<a id="lifecycle"></a>

### ライフサイクル

| 段階 | 起きること |
| --- | --- |
| ID が決まる | Slack では、エージェントとスレッドから決まる。同じスレッドなら何度でも同じ ID になる（[ADR 0016](../../adr/app/0016-chat-entry-point.md)）。CLI では新しいセッションごとにランダム |
| 作る | 記録がなければ、ロックを取って作業領域（ターゲットの clone と、そこで切ったセッションのブランチ）を作り、記録を保存する（[作業領域とブランチ](workspace.md "在りか")）。途中で失敗したら、作りかけのディレクトリを消してから失敗を返す。記録のないまま残ると、同じ ID でのやり直し（同じスレッドの次の依頼）の clone がそれにぶつかるため |
| 動かす | 依頼が来るとセッションのワーカーができ、ロックを取る。コンテナがなければ起動し、Claude Code を、会話の履歴があれば `--resume <ID>`、なければ `--session-id <ID>` で動かし続ける（[ADR 0017](../../adr/app/0017-long-lived-session-process.md)、[0048](../../adr/app/0048-resume-when-the-conversation-is-saved.md)）。ターンを終えるたびに記録の `turns` を増やして保存する |
| 休む | ターンも待っている依頼もバックグラウンドの処理もない状態が `sessions.idle_timeout` 続くと、コンテナを止めて消し、ロックを放す。その状態で今すぐの片付けが消すことになったときは、その場で休んでから片付ける（`Dispatcher.Release`、[片付け](cleanup.md "在りか")）。記録、作業領域、会話の履歴は残り、次の依頼で `--resume` する |
| 中断する | control-plane の再起動で、動いていた処理は中断され、コンテナは消され、スレッドにそう知らせる（[ADR 0024](../../adr/app/0024-restart-abandons-running-work.md)）。次の依頼で `--resume` する |
| 片付ける | 使われないまま保持期間を過ぎたか、人が頼んだセッションを判定し、消すことになれば丸ごと消す（何を残すかは[片付け](cleanup.md "在りか")） |

### セッションが持つもの

| もの | 場所 |
| --- | --- |
| 記録とロック | `<sessions.root>/<ID>/` の `session.json` と `.lock` |
| 作業領域 | `<sessions.root>/<ID>/workspace/<ターゲットのディレクトリ名>`（コンテナでは `/workspace/<ターゲットのディレクトリ名>`）（[作業領域とブランチ](workspace.md#prepare "在りか")） |
| 会話の履歴 | エージェントの Claude Code の設定ディレクトリ（[コンテナ](containers.md#claude-config "在りか")）の `projects/` の下。作業ディレクトリのパスと ID で決まる |
| 認証情報 | `acp-credentials` ボリュームの `<ID>/`（[認証情報](credentials.md#session-credentials "在りか")） |
| ブランチと PR | リモートにあるセッションが作ったブランチと、push したほかのブランチ（[作業領域とブランチ](workspace.md#branch "在りか")） |

これらは、ID でつながる。記録は ID から作業領域のパスとエージェントを引き、会話の履歴とブランチは ID から名前が
決まる。依頼を受けるのは[ワーカー](#worker)、消すのは[片付け](cleanup.md "在りか")で、どちらも[ロック](#lock)を取ってから
記録を読み直す。

<a id="invariants"></a>

### 前提

セッションは、次の前提の上に成り立っている。

- **記録が消えても ID は変わらない。** スレッドから ID が決まるので、記録だけが消えると、次の依頼は同じ ID で
  新しいセッションを作る。そのとき同じ ID の会話の履歴が残っていると、新しいセッションが消えたセッションの古い会話を
  `--resume` で引き継ぎ（下の、会話の履歴が `--session-id` と `--resume` を切り替える前提）、同じ名前のブランチがリモートに残っていると push がぶつかる。そのため、片付けは
  記録、会話の履歴、ブランチを一緒に消す。
- **作業ディレクトリのパスはセッションの間変わらない。** 会話の履歴は作業ディレクトリのパスで引かれるので、
  変わると `--resume` できない。
- **会話の履歴があるかが `--session-id` と `--resume` を切り替える。** Claude Code は、会話の履歴がある ID を
  `--session-id` で始めることも、履歴がない ID を `--resume` することも断る。履歴は最初の依頼が届いたときにでき、
  Claude Code の `cleanupPeriodDays`（既定 30 日）を過ぎると消えるので、`turns` とは合わないことがある（最初のターンの
  途中で落ちた、長く使わなかった）。そのため、エージェントの設定ディレクトリに会話の履歴のファイル
  （`projects/<ディレクトリ>/<ID>.jsonl`）があるかで決める
  （[ADR 0048](../../adr/app/0048-resume-when-the-conversation-is-saved.md)）。そのファイルに会話の行が
  書かれる前に落ちると、会話の行のないファイルが残り、どちらでも断られるので、そのセッションのプロセスは
  起動できない（片付けて、新しいセッションとして始め直す）。
- **セッションのディレクトリを触る処理は、どれもロックを取る。** 取るのは、セッションを作る処理、ワーカー、
  起動のときに残ったコンテナのセッションの `reported_usage` を消す処理（`Dispatcher.Recover`）、片付け。
  同じ control-plane の中では、同時には一つだけが触る。別の control-plane との間は、flock が効くファイルシステムの
  上でだけ守る。ロックを取れなかった処理は待たずに諦める（いつ持つか、どう守るかは[ロック](#lock)）。

## 構成要素

<a id="session-id"></a>

### セッション ID（`internal/domain` の `Conversation.SessionID`、`NewRandomSessionID`）

- **責務**：依頼がどのセッションに属するかを決める。
- **置くもの / 置かないもの**：スレッドからの ID は、エージェント名、チャットの種類（`slack`）、Slack のワークスペースの ID、
  チャンネル、スレッドの ts をつないだ名前から UUIDv5 で作る。スレッドを持たない依頼（CLI）は UUIDv4 のランダム。どちらも
  小文字の UUID で、Claude Code のセッション ID にそのまま使う。ID とスレッドの対応表は持たない。
- **関係**：セッションのブランチの名前は ID から決まる（[作業領域とブランチ](workspace.md#branch "在りか")）。
- **当てはめた原則・パターン**：[SSOT](../../../platform/plugins/principles/docs/reference/principles.md#ssot)：スレッドから毎回計算するので、
  ID の持ち主はスレッドだけで、対応表と食い違わない。その代わり、記録が消えても同じ ID が戻ってくる
  （[前提](#invariants)）。

<a id="record"></a>

### 記録（`internal/adapter/filesession`、`<sessions.root>/<ID>/session.json`）

- **責務**：セッションを続けるのに要る事実を、ID ごとに一つのファイルに持つ。
- **置くもの / 置かないもの**：次の項目。一時ファイルに書いてから名前を変えて保存する。動いているかどうか
  （コンテナ、ターン）は置かない。

  | 項目 | 中身 |
  | --- | --- |
  | `id` | セッション ID |
  | `agent` | エージェント名。別のエージェントの ID で依頼されたら断る |
  | `workspace_path` | ホストでの作業領域のパス |
  | `conversation` | 始まったスレッド（`platform` はチャットの種類、`workspace` は Slack のワークスペースの ID、`channel`、`thread`）。スレッドで始まっていなければ書かない。片付けの知らせの宛先 |
  | `turns` | 終えたターンの数。次の起動の仕方は決めない（会話の履歴で決まる。[前提](#invariants)） |
  | `last_used_at` | 最後に使った時刻。作ったとき、依頼を受けてロックを取ったとき、ターンを始めたときと終えたときなど、作業が変わりうる時点に書く（[セッションのワーカー](#worker)）。片付けの期限と、強制の片付けで知らせたあとに使われたかを見るのに使う |
  | `kept_notified_at` | 保持期間による片付けで残したことを、最後にスレッドに知らせた時刻 |
  | `reported_usage` | 最後のターンの終わりに Claude Code が報告したセッションの合計（費用 `cost_usd` と、モデルごとのトークン数と費用 `models`）。次のターンの使用量を出すのに使う。分からないときは書かない（[使用量の記録](#usage)） |

- **関係**：usecase の `SessionRepository`（`internal/usecase/ports.go`）を実装する。
- **当てはめた原則・パターン**：[層に分けた構成](../../../platform/plugins/principles/docs/reference/patterns.md#layered-architecture)：ファイルの形式は
  アダプターだけが知り、usecase は `domain.Session` だけを扱う。

<a id="lock"></a>

### ロック（`internal/adapter/filesession` の `Lock`、`<sessions.root>/<ID>/.lock`）

- **責務**：一つのセッションのディレクトリを、同時に一つの処理だけが触るようにする。
- **置くもの / 置かないもの**：ストア（`acp serve` が一つだけ作り、Dispatcher と片付けが共有する）の中の、セッションの ID ごとの持ち主と、
  `.lock` への flock の二重（[ADR 0035](../../adr/app/0035-session-lock-held-in-store.md)）。ストアの中のロックは、flock が取り合いに
  ならない bind mount（Docker Desktop の fakeowner）の上でも守る。flock は、ほかのプロセスとの間を守る。どちらも
  待たずに取りにいき、取れなければセッションが使われていると返す。プロセスが落ちればロックは外れる。セッションの
  ディレクトリも作るので、記録を作る前に取れる。取ったあと、取ったファイルが今の `.lock` と同じかを確かめ、違えば
  （ディレクトリが移されていれば）取り直す。ディレクトリを移すと（片付けか、作成に失敗した `Dispatcher` が消すとき）、
  その ID は手放したことになり、古い持ち主がロックを外しても新しい持ち主は外れない。
- **関係**：持つのは、セッションを作る間（`Dispatcher`。失敗したら作りかけのディレクトリを消して手放す）、[ワーカー](#worker)が動いている間（最初の依頼から休むまで）、
  起動のときの `Dispatcher.Recover` が、残ったコンテナのセッションの `reported_usage` を消す間、
  [片付け](cleanup.md#cleaner "在りか")がセッションや記録のないディレクトリを消す間と `kept_notified_at` を書く間。取れなければ、
  依頼はセッションが使用中だと返し、片付けはそのセッションをその回は飛ばす。ロックを取ったら、記録を読み直してから使う。
- **当てはめた原則・パターン**：[仕組みで守る](../../../platform/plugins/principles/docs/reference/principles.md#mechanisms)：同時に触らないことを
  呼び出し側の気遣いではなく、ストアのロック（ストアの中のロックと flock）で守る。[多層の防御](../../../platform/plugins/principles/docs/reference/principles.md#defense-in-depth)：
  ストアの中のロックと flock を重ね、片方が効かない所でももう片方で守る。

<a id="worker"></a>

### セッションのワーカー（`internal/usecase/dispatcher.go` の `worker`）

- **責務**：動いている一つのセッションの、プロセス、依頼の順番（届いた依頼を待たせるか流し込むかに分け、ターンの
  結果をそれを待つ依頼に返すこと）、ターンの記録を持つ（[ADR 0017](../../adr/app/0017-long-lived-session-process.md)）。
- **置くもの / 置かないもの**：
  - **できるときと記録**：最初の依頼でできる。ロックを取って記録を読み直し、`last_used_at` を書いて保存する。
    読み直したときに記録が消えていれば（片付けと入れ違えば）、依頼を断る。作られていなければ作るよう頼まれた
    依頼（`CreateIfMissing`。`HandleMessage` が付ける）なら、`Dispatcher.Execute` が同じ ID でセッションを作り直して
    から送り直し、ほかはセッションがないと返す。
  - **依頼の順番**：
    - 依頼はワーカーの受信箱（32 件まで）を通って待ち行列に入り、ターンがなければすぐに、あれば終わってから
      一つずつ送る。ワーカーはふだん受信箱からすぐ待ち行列へ移すので、待たせる依頼の数に上限はない。ワーカーが
      受け取れない間（エージェントを起動しているときなど）に受信箱が 32 件を超えると、その依頼は断る。
      依頼できるのは `allowed_users` の人と API トークンを持つ人だけなので、待ち行列に上限は持たせない。
      バックグラウンドの処理だけが残っているときは待たせない。
    - 流し込む依頼は、ターンの実行中ならすぐに送る。実行中のターンがなければ、普通の依頼と同じに扱う。
    - 送った依頼は、エージェントが取り込んだ時点（Claude Code が依頼を出力し直したとき）のターンに属する。
      一つのターンに複数の依頼が入ったら、最初の依頼が回答を受け取り、ほかは答えを兼ねたことを受け取る。
    - ターンのない間に依頼を送ったとき、またはターンが取り込まれなかった依頼を残して終わったときは、その時点で
      依頼のターンを始める。依頼が取り込まれる前にエージェントが自分のターンを終えたとき（再開したセッションが、
      前のプロセスで終わらなかったバックグラウンドの処理を知らせるときなど）は、そのターンを伝えず、依頼のターンが
      続いているものとする。そのターンの回答とエラーは、依頼のターンの回答が置き換える。エージェントが自分で
      始めたターンの途中に流し込んだ依頼が、そのターンに取り込まれなかったときは、そのターンを伝え、依頼のターンは
      そのあと始まる（[ADR 0031](../../adr/app/0031-agent-turns-before-sent-prompts.md)）。
    - セッションの停止（HTTP API と CLI）では、待っている依頼とまだ取り込まれていない依頼を取り消し、バックグラウンドの
      処理を止め、実行中のターンを中断する。ワーカーが受け取れない間に受信箱に入った依頼は、待ち行列に移る前なら
      取り消さず、あとで送ることがある。何も動いていないセッションには何もしない。どちらの停止でも、ターンのない間に
      サブエージェントの処理を止めると、Claude Code が止まったことを伝えるターンを始め、ワーカーはそれもエージェントが
      自分で始めたターンとして伝える（[ADR 0030](../../adr/app/0030-turns-after-stopping-background-work.md)）。
    - 依頼ごとの停止（Slack）では、依頼の段階で分ける。受信箱か待ち行列で待っている依頼は、それだけを取り消す。ただし、
      実行中のターンがあるときに受信箱に残っていた流し込む依頼は、止める前にエージェントに送るので、エージェントに
      渡した依頼として扱う。エージェントに渡した依頼（送ったがまだ取り込まれていない、実行中のターンに取り込まれた）は、エージェントの作業（実行中の
      ターン、バックグラウンドの処理、まだ取り込まれていない依頼）を止め、待っているほかの依頼は残す。終わった
      依頼は、エージェントが依頼なしで動いている（自分で始めたターンでまだ依頼を取り込んでいない、ターンの間に
      バックグラウンドの処理が残る）ときだけ、その作業を止め、送ったがまだ取り込まれていない依頼も取り消す。依頼を
      取り込む前のターン（上の ADR 0031 の場面）は依頼のターンなので止めない
      （[ADR 0039](../../adr/app/0039-stop-by-reacting-to-request-messages.md)）。
  - **ターンの記録**：ターンを終えるたびに `turns` を増やし、`last_used_at` と `reported_usage` を書いて保存し、
    ターンの使用量を残す（[使用量の記録](#usage)）。ターンを始めたとき、バックグラウンドの処理が終わったとき
    （止めたときも）、ターンかバックグラウンドの処理の途中でプロセスを止めたとき（control-plane が止まって中断した
    ときも）も、`last_used_at` を書いて保存する。作業が変わりうる時点をすべて記録に残し、強制の片付けが知らせた
    あとの作業を消さないようにするため（[ADR 0046](../../adr/app/0046-clean-a-session-now-by-reacting.md)）。
  - **プロセスと認証情報**：コンテナは要るときに起動し、動いている間は認証情報を一定間隔で発行し直す
    （[認証情報](credentials.md "在りか")の動いている間の更新）。コンテナの起動と Claude Code の動かし方は置かない
    （[コンテナ](containers.md "在りか")）。
  - **休む**：休める状態が `sessions.idle_timeout` 続くと、プロセスを閉じてロックを放し、なくなる。
- **関係**：`Dispatcher` がセッションごとに一つ持つ。`Dispatcher` 自身は、ワーカーの一覧と受付のほか、セッションを
  作ること、停止の受け口（`Stop`、`StopRequest`）、`Release`、`Close`、`Recover` を持つ。usecase のポート
  （`SessionRepository`、`Runner`、`CredentialIssuer` など）だけに依存する。エージェントが依頼を取り込んだことと、流し込んだときに実行中だったターンに取り込まれたかを、依頼した側に `RunAgentInput.Taken` で伝える。
  進み具合（ターンの始まりと終わり、今の作業、エージェントの発言、待っている依頼とバックグラウンドの処理の数の
  変化）を `SessionObserver` に順に伝える（[依頼と知らせ](requests.md "在りか")）。
- **当てはめた原則・パターン**：[高凝集・疎結合](../../../platform/plugins/principles/docs/reference/principles.md#cohesion)：一つのセッションの
  状態を一つのゴルーチンに集め、ほかからはチャネルで依頼と停止だけを渡す。
  [単一の情報源](../../../platform/plugins/principles/docs/reference/principles.md#ssot)：セッションの状態は
  ワーカーだけが持ち、状態メッセージはそこから伝えられた値だけを見せる。

<a id="usage"></a>

### 使用量の記録（`internal/domain/usage.go` の `TurnUsage`、`internal/adapter/usagelog`）

- **責務**：ターンごとに、誰の依頼で、どのエージェントの、どのセッションのターンが、どれだけ使ったかを残す。
- **置くもの / 置かないもの**：
  - **ターンの使用量**：Claude Code が `result` で報告するセッションの合計（`total_cost_usd` と `modelUsage`、
    サブエージェントの分を含む）から、記録の `reported_usage` を引いて出す。最初のターンの前は 0 とする。最初のターンの途中で落ちたあとも、終えた
    ターンが 0 なので 0 と比べ、落ちたプロセスが合計を保存していれば、落ちたターンの分も次のターンに入る。合計のどれかが
    前より小さければ数え直した（`/clear`）とみなし、報告された合計をそのターンの分とする。0 だけの報告（Claude Code が
    落ちたとき）は、前の合計を持ち越す（前の合計が分からなければ、分からないままにする）。費用は Claude Code の
    見積もりで、請求の値ではない。この数え方は、`--resume` したプロセスが前のプロセスの合計から数え続ける Claude Code（v2.1.277 以降。
    [ADR 0036](../../adr/app/0036-record-usage-per-turn.md)）を前提にする。
  - **分からないとき**：`reported_usage` がないと、そのターンの使用量は分からないとし、報告された合計を次と比べる値に
    する（0 だけの報告なら、分からないままにする）。ワーカーは、プロセスが自分で終わったときと、閉じても時間内に終わらず止めたときに、`Recover` は残った
    コンテナを消したときに、`reported_usage` を消す。Claude Code が合計を保存していないかもしれないため。
  - **残す先**：`usage_log`（[acp.example.yaml](../../../acp.example.yaml "在りか")）のファイルに、ターンごとに JSON を 1 行足す。
    片付けで消えないよう、セッションの外に置く。書けなければ control-plane のログに残し、ターンは続ける。
    ターンの出力は残さない。`result` の出なかったターン（control-plane が止まった、認証情報を更新できずにプロセスを
    閉じたなどで、途中で終わった）は行にならず、その分は、Claude Code が合計を保存していれば同じセッションの次の
    ターンに含まれる。

    | 項目 | 中身 |
    | --- | --- |
    | `finished_at`、`started_at` | ターンを終えた時刻と始めた時刻（UTC） |
    | `agent`、`session` | エージェント名とセッション ID |
    | `conversation` | セッションが始まったスレッド（記録の `conversation` と同じ形）。スレッドで始まっていなければ書かない |
    | `requesters` | 依頼者（`platform` と `id`）。エージェントが自分で始めたターンは、続けている依頼の依頼者。新しいプロセスが依頼を取り込む前に自分で始めたターンは、その依頼の依頼者。`platform` と `id` の順に並べる |
    | `agent_started` | エージェントが自分で始めたターンか |
    | `outcome` | `succeeded`、`error`、`usage_limited`（使用上限で断られた）、`interrupted`（止めた） |
    | `usage` | ターンの使用量（`cost_usd` と、モデルごとの `input_tokens`、`output_tokens`、`cache_read_input_tokens`、`cache_creation_input_tokens`、`cost_usd`）。分からなければ `null` |
    | `reported_total` | Claude Code が報告したセッションの合計（`usage` と同じ形） |
    | `context_tokens` | ターンの中でメインのエージェントの最後の API の呼び出しが読んだトークン数（コンテキストの大きさ。assistant のメッセージの `message.usage` の `input_tokens`、`cache_read_input_tokens`、`cache_creation_input_tokens` の和）。分からなければ `null`（[ADR 0049](../../adr/app/0049-suggest-closing-when-the-context-grows.md)） |

- **関係**：ワーカーが、ターンを終えたときに usecase の `UsageLog`（`internal/usecase/ports.go`）に `TurnRecord` を渡す。
  ターンの使用量と、次と比べるセッションの合計（0 だけの報告なら前の合計、分からなければなし）を、
  `SessionEvent.Spent` で observer にも渡し、状態メッセージが見せる
  （[依頼と知らせ](requests.md "在りか")、[ADR 0037](../../adr/app/0037-usage-in-status-messages.md)）。`result` を
  読むのは runner（`internal/adapter/docker/process.go`）で、コンテキストの大きさも runner が assistant のメッセージ
  から読み、`TurnResult.ContextTokens` で渡す。こう決めた理由は [ADR 0036](../../adr/app/0036-record-usage-per-turn.md) にある。
- **当てはめた原則・パターン**：[事実と判定を分ける](../../../platform/plugins/principles/docs/reference/patterns.md#facts-and-judgement)：
  runner は報告された合計だけを返し、ターンの分を出す判定は domain の `TurnUsage` の 1 か所に置く。
  [失われうるものを先に挙げる](../../../platform/plugins/principles/docs/reference/patterns.md#loss-first)：片付けで消えないよう、
  セッションの外に残す。
