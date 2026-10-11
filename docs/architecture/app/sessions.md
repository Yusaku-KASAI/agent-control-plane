# セッション

セッションが作られてから片付けられるまでと、セッションが持つもの。セッションと作業領域の意味は
[用語集](../../../platform/docs/reference/glossary.md)のとおり。エージェントから見たセッション（渡されるもの、
コンテナの寿命）は[ハーネスの契約](../../../platform/docs/reference/harness.md)にある。作業領域とブランチの
仕組みは[作業領域とブランチ](workspace.md)、コンテナは[コンテナ](containers.md)、認証情報は[認証情報](credentials.md)
にある。

<a id="lifecycle"></a>

## ライフサイクル

| 段階 | 起きること |
| --- | --- |
| ID が決まる | Slack では、エージェントとスレッドから決まる。同じスレッドなら何度でも同じ ID になる（[ADR 0016](../../adr/app/0016-chat-entry-point.md)）。CLI では新しいセッションごとにランダム |
| 作る | 記録がなければ、ロックを取って作業領域（ターゲットの clone と、そこで切ったセッションのブランチ）を作り、記録を保存する（[作業領域とブランチ](workspace.md)） |
| 動かす | 依頼が来るとセッションのワーカーができ、ロックを取る。コンテナがなければ起動し、Claude Code を、終えたターンが 0 なら `--session-id <ID>`、それ以外は `--resume <ID>` で動かし続ける（[ADR 0017](../../adr/app/0017-long-lived-session-process.md)）。ターンを終えるたびに記録の `turns` を増やして保存する |
| 休む | ターンも待っている依頼もバックグラウンドの処理もない状態が `sessions.idle_timeout` 続くと、コンテナを片付けてロックを放す。記録、作業領域、会話の履歴は残り、次の依頼で `--resume` する |
| 中断する | control-plane の再起動で、動いていた処理は中断され、コンテナは片付けられ、スレッドにそう知らせる（[ADR 0024](../../adr/app/0024-restart-abandons-running-work.md)）。次の依頼で `--resume` する |
| 片付ける | 使われないまま `sessions.retention`（既定値は [acp.example.yaml](../../../acp.example.yaml)）を過ぎたセッションを、control-plane が起動時と 1 時間ごとに消す。最後に使った時刻は記録の `last_used_at`。続きがありうる（開いている PR がある）か、消すと作業が失われる（作業ツリーの変更、GitHub のどこにもないコミットなど）なら残し、保持期間ごとに 1 回スレッドで片付けを促す。消すときはスレッドに何を消したかを知らせる（[ADR 0026](../../adr/app/0026-clean-up-unused-sessions.md)） |

## セッションが持つもの

| もの | 場所 |
| --- | --- |
| 記録とロック | `<sessions.root>/<ID>/` の `session.json` と `.lock` |
| 作業領域 | `<sessions.root>/<ID>/workspace/<ターゲットのディレクトリ名>`（コンテナでは `/workspace/<ターゲットのディレクトリ名>`）（[作業領域とブランチ](workspace.md#prepare)） |
| 会話の履歴 | エージェントの Claude Code の設定ディレクトリ（[コンテナ](containers.md#claude-config)）の `projects/` の下。作業ディレクトリのパスと ID で決まる |
| 認証情報 | `acp-credentials` ボリュームの `<ID>/`（[認証情報](credentials.md#session-credentials)） |
| ブランチと PR | リモートのセッションのブランチと、push したほかのブランチ（[作業領域とブランチ](workspace.md#branch)） |

これらは、ID でつながる。記録は ID から作業領域のパスとエージェントを引き、会話の履歴とブランチは ID から名前が
決まる。依頼を受けるのは[ワーカー](#worker)、消すのは[片付け](#cleaner)で、どちらも[ロック](#lock)を取ってから
記録を読み直す。

<a id="invariants"></a>

### 壊してはいけない前提

セッションに関わる変更では、次の前提が崩れないことを確かめる。

- **記録が消えても ID は変わらない。** スレッドから ID が決まるので、記録だけを消すと、次の依頼は同じ ID で新しい
  セッションを作る。そのとき、同じ ID の会話の履歴が残っていると `--session-id` が使えず、同じ名前のブランチが
  リモートに残っていると push がぶつかる。記録、会話の履歴、ブランチは一緒に扱う。
- **作業ディレクトリのパスはセッションの間変わらない。** 会話の履歴は作業ディレクトリのパスで引かれるので、
  変わると `--resume` できない。
- **`turns` は `--session-id` と `--resume` の切り替えに使う。** ターンを終えたのに保存されないと、次の起動で
  同じ ID の会話を新しく始めようとする。
- **セッションのディレクトリを触るときはロックを取る。** ロックはセッションを作る間と、ワーカーが動いている間
  （休むまで）持たれ、別の control-plane や片付けの処理と同時に触らないためにある。

## 構成要素

<a id="session-id"></a>

### セッション ID（`internal/domain` の `Conversation.SessionID`、`NewRandomSessionID`）

- **責務**：依頼がどのセッションに属するかを決める。
- **置くもの / 置かないもの**：スレッドからの ID は、エージェント名、プラットフォーム、ワークスペース、チャンネル、
  スレッドの ts をつないだ名前から UUIDv5 で作る。スレッドを持たない依頼（CLI）は UUIDv4 のランダム。どちらも
  小文字の UUID で、Claude Code のセッション ID にそのまま使う。ID とスレッドの対応表は持たない。
- **関係**：セッションのブランチの名前は ID から決まる（[作業領域とブランチ](workspace.md#branch)）。
- **当てはめた原則・パターン**：[SSOT](../../../platform/plugins/principles/docs/reference/principles.md#ssot)：スレッドから毎回計算するので、
  ID の持ち主はスレッドだけで、対応表と食い違わない。その代わり、記録が消えても同じ ID が戻ってくる
  （[壊してはいけない前提](#invariants)）。

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
  | `conversation` | 始まったスレッド（`platform`、`workspace`、`channel`、`thread`）。スレッドで始まっていなければ書かない。片付けの知らせの宛先 |
  | `turns` | 終えたターンの数。0 なら次の起動は `--session-id`、それ以外は `--resume` |
  | `last_used_at` | 最後に使った時刻。作ったとき、依頼を受けてロックを取ったとき、ターンを終えたときに書く。片付けの期限に使う |
  | `kept_notified_at` | 片付けで残したことを最後にスレッドに知らせた時刻 |

- **関係**：usecase の `SessionRepository`（`internal/usecase/ports.go`）を実装する。
- **当てはめた原則・パターン**：[依存の向き](../../../platform/plugins/principles/docs/reference/principles.md#dependency-rule)：ファイルの形式は
  アダプタだけが知り、usecase は `domain.Session` だけを扱う。

<a id="lock"></a>

### ロック（`internal/adapter/filesession` の `Lock`、`<sessions.root>/<ID>/.lock`）

- **責務**：一つのセッションのディレクトリを、同時に一つの処理だけが触るようにする。
- **置くもの / 置かないもの**：`.lock` への flock。待たずに取りにいき、取れなければセッションが使われていると
  返す。プロセスが落ちればロックは外れる。セッションのディレクトリも作るので、記録を作る前に取れる。
  取ったあと、取ったファイルが今の `.lock` と同じかを確かめ、違えば（片付けがディレクトリを移していれば）
  取り直す。
- **関係**：持つのは、セッションを作る間、[ワーカー](#worker)が動いている間、[片付け](#cleaner)が消す間と
  `kept_notified_at` を書く間。ロックを取ったら、記録を読み直してから使う。
- **当てはめた原則・パターン**：[仕組みで守る](../../../platform/plugins/principles/docs/reference/principles.md#mechanisms)：同時に触らないことを
  呼び出し側の気遣いではなく OS のロックで守る。

<a id="worker"></a>

### セッションのワーカー（`internal/usecase/dispatcher.go` の `worker`）

- **責務**：動いている一つのセッションの、プロセス、依頼の順番、ターンの記録を持つ。
- **置くもの / 置かないもの**：
  - 最初の依頼でできる。ロックを取って記録を読み直し、`last_used_at` を書いて保存する。読み直したときに記録が
    消えていれば（片付けと入れ違えば）、同じ ID でセッションを作り直す。
  - 依頼を受信箱で受け、順番に扱う（[依頼と知らせ](requests.md)）。コンテナは要るときに起動し、認証情報を定期的に
    発行し直す（[認証情報](credentials.md)）。
  - ターンを終えるたびに `turns` を増やし、`last_used_at` を書いて保存する。
  - 休める状態が `sessions.idle_timeout` 続くと、プロセスを閉じてロックを放し、なくなる。
  - コンテナの起動と Claude Code の動かし方は置かない（[コンテナ](containers.md)）。
- **関係**：`Dispatcher` がセッションごとに一つ持ち、`Dispatcher` 自身はワーカーの一覧と受付だけを持つ。usecase のポート（`SessionRepository`、`Runner`、
  `CredentialIssuer` など）だけに依存する。
- **当てはめた原則・パターン**：[高凝集・疎結合](../../../platform/plugins/principles/docs/reference/principles.md#cohesion)：一つのセッションの
  状態を一つのゴルーチンに集め、ほかからはチャネルで依頼と停止だけを渡す。

<a id="cleaner"></a>

### 片付け（`internal/usecase/clean_sessions.go` の `SessionCleaner`）

- **責務**：保持期間を過ぎたセッションを、残す理由がなければ消し、スレッドに知らせる。
- **置くもの / 置かないもの**：
  - **事実**：作業領域の状態（`SessionWorkspace`）と、リモートのブランチの事実（`SessionBranches`）を集める。
    問い合わせるのは、セッションの名前でリモートにあるブランチ、push したブランチ、ほかにないコミットを持つ
    ローカルのブランチ。事実の中身は[作業領域とブランチ](workspace.md)にある。
  - **理由**：事実から残す理由のリストを作るのは `reasons` の 1 か所だけ。残すか消すかと、知らせる文面は、
    そのリストから決める。状態の軸と判定の表は [ADR 0026](../../adr/app/0026-clean-up-unused-sessions.md) にある。
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
- **当てはめた原則・パターン**：[事実と判定を分ける](../../../platform/plugins/principles/docs/reference/patterns.md#facts-and-judgement)：アダプタは事実
  だけを返し、判定はここに置く。残すか消すか、何を知らせるかは理由のリスト一つから決める。
  [状態を軸で洗い出す](../../../platform/plugins/principles/docs/reference/patterns.md#state-axes)：判定は ADR 0026 の軸と判定の表に沿う。
  [失われうるものを先に挙げる](../../../platform/plugins/principles/docs/reference/patterns.md#loss-first)：消すと失われるものがあれば残す。
  [高凝集・疎結合](../../../platform/plugins/principles/docs/reference/principles.md#cohesion)：記録、会話の履歴、ブランチは一緒に
  変わるものとして一緒に消す（[壊してはいけない前提](#invariants)）。
