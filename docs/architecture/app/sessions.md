# セッション

セッションが作られてから片付けられるまでと、セッションが持つもの。セッションと作業領域の意味は
[用語集](../../../platform/docs/reference/glossary.md)のとおり。エージェントから見たセッション（渡されるもの、
コンテナの寿命）は[ハーネスの契約](../../../platform/docs/reference/harness.md "在りか")にある。作業領域とブランチの
仕組みは[作業領域とブランチ](workspace.md "在りか")、コンテナは[コンテナ](containers.md "在りか")、認証情報は[認証情報](credentials.md "在りか")
にある。

## 構成

<a id="lifecycle"></a>

### ライフサイクル

| 段階 | 起きること |
| --- | --- |
| ID が決まる | Slack では、エージェントとスレッドから決まる。同じスレッドなら何度でも同じ ID になる（[ADR 0016](../../adr/app/0016-chat-entry-point.md)）。CLI では新しいセッションごとにランダム |
| 作る | 記録がなければ、ロックを取って作業領域（ターゲットの clone と、そこで切ったセッションのブランチ）を作り、記録を保存する（[作業領域とブランチ](workspace.md "在りか")） |
| 動かす | 依頼が来るとセッションのワーカーができ、ロックを取る。コンテナがなければ起動し、Claude Code を、終えたターンが 0 なら `--session-id <ID>`、それ以外は `--resume <ID>` で動かし続ける（[ADR 0017](../../adr/app/0017-long-lived-session-process.md)）。ターンを終えるたびに記録の `turns` を増やして保存する |
| 休む | ターンも待っている依頼もバックグラウンドの処理もない状態が `sessions.idle_timeout` 続くと、コンテナを止めて消し、ロックを放す。記録、作業領域、会話の履歴は残り、次の依頼で `--resume` する |
| 中断する | control-plane の再起動で、動いていた処理は中断され、コンテナは消され、スレッドにそう知らせる（[ADR 0024](../../adr/app/0024-restart-abandons-running-work.md)）。次の依頼で `--resume` する |
| 片付ける | 使われないまま `sessions.retention`（既定値は [acp.example.yaml](../../../acp.example.yaml "在りか")）を過ぎたセッションを、control-plane が起動時と 1 時間ごとに調べ、残す理由がなければ丸ごと消す。残す理由があれば残し、保持期間ごとに 1 回スレッドで片付けを促す（[片付け](#cleaner)） |

### セッションが持つもの

| もの | 場所 |
| --- | --- |
| 記録とロック | `<sessions.root>/<ID>/` の `session.json` と `.lock` |
| 作業領域 | `<sessions.root>/<ID>/workspace/<ターゲットのディレクトリ名>`（コンテナでは `/workspace/<ターゲットのディレクトリ名>`）（[作業領域とブランチ](workspace.md#prepare "在りか")） |
| 会話の履歴 | エージェントの Claude Code の設定ディレクトリ（[コンテナ](containers.md#claude-config "在りか")）の `projects/` の下。作業ディレクトリのパスと ID で決まる |
| 認証情報 | `acp-credentials` ボリュームの `<ID>/`（[認証情報](credentials.md#session-credentials "在りか")） |
| ブランチと PR | リモートのセッションのブランチと、push したほかのブランチ（[作業領域とブランチ](workspace.md#branch "在りか")） |

これらは、ID でつながる。記録は ID から作業領域のパスとエージェントを引き、会話の履歴とブランチは ID から名前が
決まる。依頼を受けるのは[ワーカー](#worker)、消すのは[片付け](#cleaner)で、どちらも[ロック](#lock)を取ってから
記録を読み直す。

<a id="invariants"></a>

### 前提

セッションは、次の前提の上に成り立っている。

- **記録が消えても ID は変わらない。** スレッドから ID が決まるので、記録だけが消えると、次の依頼は同じ ID で
  新しいセッションを作る。そのとき同じ ID の会話の履歴が残っていると `--session-id` が使えず、同じ名前のブランチが
  リモートに残っていると push がぶつかる。そのため、片付けは記録、会話の履歴、ブランチを一緒に消す。
- **作業ディレクトリのパスはセッションの間変わらない。** 会話の履歴は作業ディレクトリのパスで引かれるので、
  変わると `--resume` できない。
- **`turns` が `--session-id` と `--resume` を切り替える。** ターンを終えるたびに保存される。保存されていないと、
  次の起動で、会話の履歴がある ID を `--session-id` で始めようとする。
- **セッションのディレクトリを触る処理は、どれもロックを取る。** 取るのはセッションを作る処理、ワーカー、片付けで、
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
  | `turns` | 終えたターンの数。0 なら次の起動は `--session-id`、それ以外は `--resume` |
  | `last_used_at` | 最後に使った時刻。作ったとき、依頼を受けてロックを取ったとき、ターンを終えたときに書く。片付けの期限に使う |
  | `kept_notified_at` | 片付けで残したことを最後にスレッドに知らせた時刻 |
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
  （片付けがディレクトリを移していれば）取り直す。片付けがディレクトリを移すと、その ID は手放したことになり、
  古い持ち主がロックを外しても新しい持ち主は外れない。
- **関係**：持つのは、セッションを作る間（`Dispatcher`）、[ワーカー](#worker)が動いている間（最初の依頼から休むまで）、
  [片付け](#cleaner)がセッションや記録のないディレクトリを消す間と `kept_notified_at` を書く間。取れなければ、
  依頼はセッションが使用中だと返し、片付けはそのセッションをその回は飛ばす。ロックを取ったら、記録を読み直してから使う。
- **当てはめた原則・パターン**：[仕組みで守る](../../../platform/plugins/principles/docs/reference/principles.md#mechanisms)：同時に触らないことを
  呼び出し側の気遣いではなく、ストアのロック（ストアの中のロックと flock）で守る。[多層の防御](../../../platform/plugins/principles/docs/reference/principles.md#defense-in-depth)：
  ストアの中のロックと flock を重ね、片方が効かない所でももう片方で守る。

<a id="worker"></a>

### セッションのワーカー（`internal/usecase/dispatcher.go` の `worker`）

- **責務**：動いている一つのセッションの、プロセス、依頼の順番、ターンの記録を持つ。
- **置くもの / 置かないもの**：
  - 最初の依頼でできる。ロックを取って記録を読み直し、`last_used_at` を書いて保存する。読み直したときに記録が
    消えていれば（片付けと入れ違えば）、同じ ID でセッションを作り直す。
  - 依頼を受信箱で受け、順番に扱う（[依頼と知らせ](requests.md "在りか")）。コンテナは要るときに起動し、認証情報を定期的に
    発行し直す（[認証情報](credentials.md "在りか")）。
  - ターンを終えるたびに `turns` を増やし、`last_used_at` と `reported_usage` を書いて保存し、ターンの使用量を残す
    （[使用量の記録](#usage)）。
  - 休める状態が `sessions.idle_timeout` 続くと、プロセスを閉じてロックを放し、なくなる。
  - コンテナの起動と Claude Code の動かし方は置かない（[コンテナ](containers.md "在りか")）。
- **関係**：`Dispatcher` がセッションごとに一つ持ち、`Dispatcher` 自身はワーカーの一覧と受付だけを持つ。usecase のポート（`SessionRepository`、`Runner`、
  `CredentialIssuer` など）だけに依存する。
- **当てはめた原則・パターン**：[高凝集・疎結合](../../../platform/plugins/principles/docs/reference/principles.md#cohesion)：一つのセッションの
  状態を一つのゴルーチンに集め、ほかからはチャネルで依頼と停止だけを渡す。

<a id="usage"></a>

### 使用量の記録（`internal/domain/usage.go` の `TurnUsage`、`internal/adapter/usagelog`）

- **責務**：ターンごとに、誰の依頼で、どのエージェントの、どのセッションのターンが、どれだけ使ったかを残す。
- **置くもの / 置かないもの**：
  - **ターンの使用量**：Claude Code が `result` で報告するセッションの合計（`total_cost_usd` と `modelUsage`、
    サブエージェントの分を含む）から、記録の `reported_usage` を引いて出す。最初のターンの前は 0 とする。合計のどれかが
    前より小さければ数え直した（`/clear`）とみなし、報告された合計をそのターンの分とする。0 だけの報告（Claude Code が
    落ちたとき）は、前の合計を持ち越す（前の合計が分からなければ、分からないままにする）。費用は Claude Code の
    見積もりで、請求の値ではない。`--resume` で合計を引き継ぐのは Claude Code v2.1.277 から。
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

- **関係**：ワーカーが、ターンを終えたときに usecase の `UsageLog`（`internal/usecase/ports.go`）に `TurnRecord` を渡す。
  ターンの使用量と、次と比べるセッションの合計（0 だけの報告なら前の合計、分からなければなし）を、
  `SessionEvent.Spent` で observer にも渡し、状態メッセージが見せる
  （[依頼と知らせ](requests.md "在りか")、[ADR 0037](../../adr/app/0037-usage-in-status-messages.md)）。`result` を
  読むのは runner（`internal/adapter/docker/process.go`）。こう決めた理由は [ADR 0036](../../adr/app/0036-record-usage-per-turn.md) にある。
- **当てはめた原則・パターン**：[事実と判定を分ける](../../../platform/plugins/principles/docs/reference/patterns.md#facts-and-judgement)：
  runner は報告された合計だけを返し、ターンの分を出す判定は domain の `TurnUsage` の 1 か所に置く。
  [失われうるものを先に挙げる](../../../platform/plugins/principles/docs/reference/patterns.md#loss-first)：片付けで消えないよう、
  セッションの外に残す。

<a id="cleaner"></a>

### 片付け（`internal/usecase/clean_sessions.go` の `SessionCleaner`）

- **責務**：保持期間を過ぎたセッションを、残す理由がなければ消し、スレッドに知らせる。
- **置くもの / 置かないもの**：
  - **事実**：作業領域の状態（`SessionWorkspace`）と、リモートのブランチの事実（`SessionBranches`）を集める。
    問い合わせるのは、セッションの名前でリモートにあるブランチ、push したブランチ、ほかにないコミットを持つ
    ローカルのブランチ。事実の中身は[作業領域とブランチ](workspace.md "在りか")にある。
  - **理由**：事実から残す理由のリストを作るのは `reasons` の 1 か所だけ。残すか消すかと、知らせる文面は、
    そのリストから決める。状態の軸と判定の表は下の[状態の軸](#cleanup-axes)と[判定](#cleanup-rules)にある。
  - **消す手順**：まずロックを取らずに判定する。消すときはロックを取り、記録を読み直して、その間に使われて
    いないことと理由がないことをもう一度確かめる。消す順は、セッションの名前でリモートにあるブランチ、認証情報と
    会話の履歴、最後に記録と作業領域（ディレクトリを `.removed-` で始まる名前に一度に移してから消す）。記録が
    残っている間は ID がこのセッションを指すので、途中で失敗しても次の回にやり直せる。記録のない
    ディレクトリは、ロックが取れれば消す。ロックが取れないセッションは、その回は飛ばす。
  - **知らせ**：消したら、何を消したかと、続けると新しいセッションになることを知らせる。残したら理由を挙げて
    促し、`kept_notified_at` が保持期間より前のときだけ知らせる。知らせはロックを持たずに投稿し、投稿できて
    から `kept_notified_at` を書く。スレッドで始まっていないセッションには知らせない。
  - いつ動かすか（[ライフサイクル](#lifecycle)）は `cmd/acp/serve.go` が持つ。
- **関係**：usecase のポート（`SessionRepository`、`SessionWorkspace`、`SessionBranches`、`SessionFiles`、
  `SessionNotifier`）だけに依存する。会話の履歴と認証情報を消すのは `SessionFiles`
  （`internal/adapter/docker/sessionfiles.go`）、記録と作業領域を消すのは `SessionRepository.Remove`。
- **当てはめた原則・パターン**：[事実と判定を分ける](../../../platform/plugins/principles/docs/reference/patterns.md#facts-and-judgement)：アダプターは事実
  だけを返し、判定はここに置く。残すか消すか、何を知らせるかは理由のリスト一つから決める。
  [状態を軸で洗い出す](../../../platform/plugins/principles/docs/reference/patterns.md#state-axes)：判定は下の軸と判定の表に沿う。
  [失われうるものを先に挙げる](../../../platform/plugins/principles/docs/reference/patterns.md#loss-first)：消すと失われるものがあれば残す。
  [高凝集・疎結合](../../../platform/plugins/principles/docs/reference/principles.md#cohesion)：記録、会話の履歴、ブランチは一緒に
  変わるものとして一緒に消す（[前提](#invariants)）。

<a id="cleanup-axes"></a>

#### 状態の軸

判定の拠り所になる状態の軸。こう決めた理由と選ばなかった案は [ADR 0026](../../adr/app/0026-clean-up-unused-sessions.md) と、ほかの worktree を足した [ADR 0042](../../adr/app/0042-judge-linked-worktrees-in-cleanup.md) にある。
軸は、それぞれの対象の上で、値が互いに重ならず、すべての場合を尽くす。括弧内の条件は、その軸が定義される範囲。
「ほかにないコミット」は、`origin` の remote-tracking ブランチにも `refs/acp/target` にもないコミット。head
ブランチ、base ブランチ、head コミットは[用語集](../../../platform/docs/reference/glossary.md)のとおり（閉じた PR の
head コミットは、閉じたときの先頭）。

セッション（対象：セッションのディレクトリ）：

| 軸 | 値 |
| --- | --- |
| 記録 | ある / ない |
| ロック | 取れる / 取れない（プロセスが動いている） |
| 期限（記録がある） | 最後に使ってから `sessions.retention` 以内 / 超過 |
| 作業領域（記録がある） | git のリポジトリ / そうでない（`.git` がない） |
| 作業ツリーの変更（git のリポジトリ） | ない / ある（主の worktree か、作業領域の中のほかの worktree に、コミットしていない変更か、ignore されていない追跡していないファイル） |
| stash（同） | ない / ある |
| 途中の操作（同） | ない / ある（主の worktree か、作業領域の中のほかの worktree で、rebase、merge、cherry-pick、revert、bisect） |
| HEAD（同。主の worktree と、作業領域の中のほかの worktree ごと） | ブランチを指す / detached で、ローカルのブランチにもないほかにないコミットを持つ / detached で、持たない |
| GitHub（記録がある） | 読める / 読めない（GitHub App がない、ターゲットの `origin` が GitHub でない） |

ブランチ（対象：ローカルのブランチ、push したブランチ、セッションの名前でリモートにあるブランチの和。名前で同じものとする）：

| 軸 | 値 |
| --- | --- |
| 名前 | セッションのもの（[セッションのブランチ](workspace.md#branch "在りか")）/ ほか |
| push の記録 | 一度でもある / ない（[作業領域の状態](workspace.md#state "在りか")） |
| ローカル | ない / あり、ほかにないコミットを持つ / あり、持たない |
| リモート（GitHub を読める） | ある / ない |
| このブランチを head ブランチとする開いている PR（GitHub を読める） | ある / ない |
| このブランチを base ブランチとする開いている PR（GitHub を読める、リモートにある） | ある / ない |

コミット（対象：ブランチとコミットの組。ほかにないコミットを持つローカルのブランチとその先頭、push したブランチと
最後に push したコミット、セッションの名前でリモートにあるブランチとその先頭）：

| 軸 | 値（上から順に、最初に当たるもの） |
| --- | --- |
| 行き先（GitHub を読める） | 既定のブランチ / 同じ名前のブランチを head ブランチとする、マージした PR の head コミット / 同じく、マージせずに閉じた PR の head コミット / リモートの同じ名前のブランチ / どこにもない |

作業領域が git のリポジトリでなければ、作業ツリーの変更・stash・途中の操作はなく、HEAD は当たらず、ローカルの
ブランチと push したブランチはないものとする。GitHub を読めるのに GitHub の API が失敗したときは、その回はその
セッションを判定しない。

<a id="cleanup-rules"></a>

#### 判定

- 記録がなく、ロックが取れる：ディレクトリを消す（作るのに失敗した残りか、片付けと入れ違った依頼が作ったロック
  だけのもの）。
- 記録があり、期限が以内：何もしない。
- 記録があり、期限が超過：次の理由のどれかに当たれば残し、理由を挙げて保持期間ごとに 1 回スレッドで知らせる。
  当たらなければ消す。
- ロックが取れなければ、記録の有無によらず、その回は消さない。理由に当たらないなら知らせもしない。

| 理由 | 状態 |
| --- | --- |
| 続きがありうる | 名前：セッションのもの、または push の記録：一度でもある。かつ、head ブランチとする開いている PR：ある |
| 消すと失われる（作業領域） | 作業ツリーの変更：ある、stash：ある、途中の操作：ある、HEAD：detached でほかにないコミットを持つ、のどれか |
| 消すと失われる（push していないコミット） | ローカル：あり、ほかにないコミットを持つ。かつ、GitHub：読めない、またはローカルの先頭の行き先：どこにもない |
| 消すと失われる（push したコミット） | push の記録：一度でもある、GitHub：読める、最後に push したコミットの行き先：どこにもない |
| 消すと失われる（消すブランチ） | 名前：セッションのもの、リモート：ある、head ブランチとする開いている PR：ない、リモートの先頭の行き先：リモートの同じ名前のブランチ |
| 消すと PR が閉じる | 名前：セッションのもの、リモート：ある、base ブランチとする開いている PR：ある |

- 名前がセッションのものかどうかは、リモートのブランチを消すかどうかと、その PR を続きとみなすかに使い、コミットが
  残っているかの判断には使わない。消さないブランチの内容はリモートに残るので、そのブランチでは、セッションの
  コミットがそこに入っているか（行き先：リモートの同じ名前のブランチ）で判断する。
- 消すのは、記録、作業領域（ローカルのブランチ、作業ツリーを含む）、認証情報、会話の履歴、セッションの名前で
  リモートにあるブランチ。リモートのほかのブランチと PR は消さない。
