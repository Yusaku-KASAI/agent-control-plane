# 片付け

使われなくなったセッションを、残す理由がなければ丸ごと消す仕組みと、残すか消すかを決めるために作業領域と GitHub
から読む事実。セッション全体は[セッション](sessions.md "在りか")、作業領域とブランチは
[作業領域とブランチ](workspace.md "在りか")、作業領域を読むコンテナと、会話の履歴と認証情報を消す部品は
[コンテナ](containers.md "在りか")にある。人の操作は[依頼の仕方](../../reference/usage.md "在りか")に、エージェントから見た
片付け（`acp cleanup-check`）は[ハーネスの契約](../../../platform/docs/reference/harness.md#lifetime "在りか")にある。

## 構成

```
serve（起動時と 1 時間ごと）／ HandleMessage（スレッドの最初のメッセージへのリアクション）
            │
            ▼
SessionCleaner ──SessionWorkspace──▶ 作業領域を読むコンテナ（acp workspace-state）
   │           ──SessionBranches───▶ GitHub のブランチと PR
   │  readFacts で事実を集め、reasons で判定する
   │
   ├─ 残す理由がない：ブランチ、会話の履歴と認証情報、記録と作業領域を消す
   └─ 残す理由がある：残して、スレッドに理由を知らせる

acp cleanup-check（エージェントのコンテナ）──▶ CheckCleanup ──▶ 同じ事実と reasons で、残す理由を返す
```

- 保持期間（`sessions.retention`、既定値は [acp.example.yaml](../../../acp.example.yaml "在りか")）を過ぎたセッションを、
  control-plane が起動時と 1 時間ごとに調べる。人はスレッドの最初のメッセージへのリアクションで、保持期間によらず
  今すぐ判定させられる（`CleanNow`）。
- 消すのは、記録、作業領域（ローカルのブランチ、作業ツリーを含む）、認証情報、会話の履歴、リモートにある
  セッションが作ったブランチで、これらは一緒に変わるので一緒に消す（[セッションの前提](sessions.md#invariants)）。
  リモートのほかのブランチと PR は消さない。

<a id="facts"></a>

### 事実と判定

作業領域と GitHub を読むアダプターは、事実（どのブランチに push したか、どのコミットがどこに入っているか）だけを
返す。残すか消すかは[片付け](#cleaner)の 1 か所が決める。アダプターの側では、どのブランチをセッションが
作ったかも、どのコミットが失われるかも決めない。

<a id="cleanup-axes"></a>

### 状態の軸

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

ブランチ（対象：ローカルのブランチ、push したブランチ、リモートにあるセッションが作ったブランチの和。名前で同じものとする）：

| 軸 | 値 |
| --- | --- |
| 名前 | セッションが作ったもの（[セッションが作ったブランチ](workspace.md#branch "在りか")）/ ほか |
| push の記録 | 一度でもある / ない（[作業領域の状態](#state)） |
| ローカル | ない / あり、ほかにないコミットを持つ / あり、持たない |
| リモート（GitHub を読める） | ある / ない |
| このブランチを head ブランチとする開いている PR（GitHub を読める） | ある / ない |
| このブランチを base ブランチとする開いている PR（GitHub を読める、リモートにある） | ある / ない |

コミット（対象：ブランチとコミットの組。ほかにないコミットを持つローカルのブランチとその先頭、push したブランチと
最後に push したコミット、リモートにあるセッションが作ったブランチとその先頭）：

| 軸 | 値（上から順に、最初に当たるもの） |
| --- | --- |
| 行き先（GitHub を読める） | 既定のブランチ / 同じ名前のブランチを head ブランチとする、マージした PR の head コミット / 同じく、マージせずに閉じた PR の head コミット / リモートの同じ名前のブランチ / どこにもない |

作業領域が git のリポジトリでなければ、作業ツリーの変更・stash・途中の操作はなく、HEAD は当たらず、ローカルの
ブランチと push したブランチはないものとする。GitHub を読めるのに GitHub の API が失敗したときは、その回はその
セッションを判定しない。

<a id="cleanup-rules"></a>

### 判定

- 記録がなく、ロックが取れる：ディレクトリを消す（作るのに失敗した残りか、片付けと入れ違った依頼が作ったロック
  だけのもの）。
- 記録があり、期限が以内：何もしない。
- 記録があり、期限が超過：次の理由のどれかに当たれば残し、理由を挙げて保持期間ごとに 1 回スレッドで知らせる。
  当たらなければ消す。
- ロックが取れなければ、記録の有無によらず、その回は消さない。理由に当たらないなら知らせもしない。
- 人が今すぐの片付けを頼んだ（`CleanNow`）：期限によらず、記録があるセッションを次のように判定し、結果をスレッドに
  返す。ロックを持たずに判定し、消すことになったときだけ、休んでいるだけのワーカーがロックを持っていれば手放させ
  （`Dispatcher.Release`）、作業中なら消さない。判定できなければ（エージェントの設定、認証情報の発行、作業領域の
  読み取り、GitHub の API、ワーカーの手放し、消す途中の失敗）、そうスレッドに知らせ、付け直しを案内する。
  投稿の失敗は投稿し直さない。記録を読めないときは、宛先が分からないので知らせない。
  - 通常（`:wastebasket:`）：理由のどれかに当たれば残し、当たらなければ消す。
  - 強制（`:fire:`）：付けた人が `:wastebasket:` で理由を知らされた本人で、知らせるために判定の始めに読んだ
    `last_used_at` が今も変わっておらず、知らせた時刻が付けた時刻の読む時間（10 秒）より前で 1 時間のうちにあり（Slack の時計。`:wastebasket:` の
    イベントの時刻に判定にかかった時間を足す）、control-plane が受け取ったのも知らせてから読む時間より後で
    （control-plane の時計）、今の理由がどれも知らせた理由にあれば、「続きがありうる」と「消すと PR が閉じる」の
    ほかの理由では残さない。ロックの下で判定し直しても同じ。そうでなければ、通常と同じく
    理由を知らせて残す。ただし、強制として受けない `:fire:` で残す理由がないときは、称賛かもしれないので消さず、
    `:wastebasket:` を案内する。
  - 本人への知らせ（誰に、いつ、どの理由を）は、記録ではなく `SessionCleaner` のメモリに 1 時間持つ。休んでいる
    ワーカーが記録のロックを持つあいだも書けるようにするため。保持期間ごとの促し、強制として受けなかった `:fire:`、
    今すぐの片付けで消したときに忘れ、再起動でも消える（[ADR 0046](../../adr/app/0046-clean-a-session-now-by-reacting.md)）。

| 理由 | 状態 |
| --- | --- |
| 続きがありうる | 名前：セッションが作ったもの、または push の記録：一度でもある。かつ、head ブランチとする開いている PR：ある |
| 消すと失われる（作業領域） | 作業ツリーの変更：ある、stash：ある、途中の操作：ある、HEAD：detached でほかにないコミットを持つ、のどれか。ただし HEAD は、その先頭が「消すと失われる（push したコミット）」に当たるブランチの最後に push したコミットのときは除く |
| 消すと失われる（push していないコミット） | ローカル：あり、ほかにないコミットを持つ。かつ、GitHub：読めない、またはローカルの先頭の行き先：どこにもない。ただし、ローカルの先頭が、「消すと失われる（push したコミット）」に当たるブランチ（名前は違ってよい）の最後に push したコミットのときは除く |
| 消すと失われる（push したコミット） | push の記録：一度でもある、GitHub：読める、最後に push したコミットの行き先：どこにもない |
| 消すと失われる（消すブランチ） | 名前：セッションが作ったもの、リモート：ある、head ブランチとする開いている PR：ない、リモートの先頭の行き先：リモートの同じ名前のブランチ |
| 消すと PR が閉じる | 名前：セッションが作ったもの、リモート：ある、base ブランチとする開いている PR：ある |

- セッションが作ったブランチかどうかは、リモートのブランチを消すかどうかと、その PR を続きとみなすかに使い、コミットが
  残っているかの判断には使わない。消さないブランチの内容はリモートに残るので、そのブランチでは、セッションの
  コミットがそこに入っているか（行き先：リモートの同じ名前のブランチ）で判断する。

## 構成要素

<a id="cleaner"></a>

### 片付け（`internal/usecase/clean_sessions.go` の `SessionCleaner`）

- **責務**：保持期間を過ぎたセッションを、残す理由がなければ消し、スレッドに知らせる。人がスレッドで頼んだときは、
  保持期間によらずそのセッションを判定する（`CleanNow`）。
- **置くもの / 置かないもの**：
  - **事実**：作業領域の状態（`SessionWorkspace`）と、リモートのブランチの事実（`SessionBranches`）を集める。
    問い合わせるのは、リモートにあるセッションが作ったブランチ、push したブランチ、ほかにないコミットを持つ
    ローカルのブランチ。事実の中身は下の[作業領域の状態](#state)と[GitHub のブランチ](#github-branches)にある。
  - **理由**：事実から残す理由のリストを作るのは `reasons` の 1 か所だけ。残すか消すかと、知らせる文面は、
    そのリストから決める。状態の軸と判定の表は上の[状態の軸](#cleanup-axes)と[判定](#cleanup-rules)にある。
  - **消す手順**：まずロックを取らずに判定する。消すときはロックを取り、記録を読み直して、その間に使われて
    いないことと理由がないことをもう一度確かめる。消す順は、リモートにあるセッションが作ったブランチ、認証情報と
    会話の履歴、最後に記録と作業領域（ディレクトリを `.removed-` で始まる名前に一度に移してから消す）。記録が
    残っている間は ID がこのセッションを指すので、途中で失敗しても次の回にやり直せる。記録のない
    ディレクトリは、ロックが取れれば消す。ロックが取れないセッションは、その回は飛ばす。
  - **知らせ**：消したら、何を消したかと、続けると新しいセッションになることを知らせる。残したら理由を挙げて
    促し、`kept_notified_at` が保持期間より前のときだけ知らせる。知らせはロックを持たずに投稿し、投稿できて
    から `kept_notified_at` を書く。スレッドで始まっていないセッションへの知らせは、
    [sessionObserver](requests.md#observer "在りか") が捨てる。
  - いつ動かすかは `cmd/acp/serve.go` が持つ。
  - **今すぐの片付け**：`CleanNow` は、スレッドの最初のメッセージへのリアクションで、そのセッションを保持期間に
    よらず判定し、結果をスレッドに返す。返すのは、消したこと、残した理由、作業中なので片付けないこと、片付ける間に
    使われたので消していないこと、判定できなかったこと、強制として受けない `:fire:` で残す理由がないときの
    `:wastebasket:` の案内。通常と強制で何を残すかは上の[判定](#cleanup-rules)にあり、
    強制で残す理由を選ぶのは `keeps`（開いている PR の理由は、消すと PR が閉じるか、レビューに応えるセッションが
    なくなるので、強制でも残す）。消す手順は保持期間による片付けと同じで、ロックの下で判定し直し、その間に
    使われていれば消さずにそう知らせる（[ADR 0046](../../adr/app/0046-clean-a-session-now-by-reacting.md)）。
  - 残した知らせ（保持期間ごとの促しと、今すぐの片付けの返事）は、エージェントに「締めて」と頼むことと、
    `:wastebasket:` での片付けを案内する。
- **関係**：usecase のポート（`AgentRegistry`、`SessionRepository`、`CredentialIssuer`、`SessionWorkspace`、
  `SessionBranches`、`SessionFiles`、`SessionNotifier`、`SessionHolder`）だけに依存する。休んでいるワーカーを手放させるのは `SessionHolder`
  （`Dispatcher.Release`）。作業領域を読むのは `SessionWorkspace`（`internal/adapter/docker/workspace.go`。
  作業領域を読むコンテナの中で読む）、会話の履歴と認証情報を消すのは `SessionFiles`
  （`internal/adapter/docker/sessionfiles.go`）、記録と作業領域を消すのは `SessionRepository.Remove`。
- **当てはめた原則・パターン**：[事実と判定を分ける](../../../platform/plugins/principles/docs/reference/patterns.md#facts-and-judgement)：アダプターは事実
  だけを返し、判定はここに置く。残すか消すか、何を知らせるかは理由のリスト一つから決める。
  [状態を軸で洗い出す](../../../platform/plugins/principles/docs/reference/patterns.md#state-axes)：判定は上の軸と判定の表に沿う。
  [失われうるものを先に挙げる](../../../platform/plugins/principles/docs/reference/patterns.md#loss-first)：消すと失われるものがあれば残す。
  [高凝集・疎結合](../../../platform/plugins/principles/docs/reference/principles.md#cohesion)：記録、会話の履歴、ブランチは一緒に
  変わるものとして一緒に消す（[セッションの前提](sessions.md#invariants)）。

<a id="check-cleanup"></a>

### 判定を外から見る（`internal/usecase/check_cleanup.go` の `CheckCleanup`）

- **責務**：一つの作業領域について、保持期間によらず残す理由を種類と文面で返す。種類は判定の表の理由ごとで、
  作業領域の理由は変更、stash、途中の操作、detached HEAD に分ける
  （[ADR 0047](../../adr/app/0047-cleanup-check-reads-the-given-workspace-and-splits-lost-pushes.md)）。
- **関係**：`SessionCleaner` と同じ事実の集め方（`readFacts`）と判定（`reasons`）を使い、ポートは `SessionWorkspace` と
  `SessionBranches` だけに依存する。呼ぶのは、エージェントのコンテナの中で動く `acp cleanup-check`
  （`cmd/acp/cleanup_check.go`。[ハーネスの契約](../../../platform/docs/reference/harness.md#lifetime "在りか")）。
- **当てはめた原則・パターン**：[単一の情報源](../../../platform/plugins/principles/docs/reference/principles.md#ssot)：
  エージェントが見る理由と片付けが残す理由は、同じ `reasons` から作る。

<a id="state"></a>

### 作業領域の状態（`internal/adapter/gitworkspace/provider.go` の `State`）

- **責務**：作業領域が、リモートにないものとして何を持っているかを読む。
- **置くもの / 置かないもの**：次の事実を返す。作業領域がないか git のリポジトリでなければ、何も返さない。

  | 事実 | 読み方 |
  | --- | --- |
  | 作業ツリーの変更 | 主の worktree か、作業領域の中のほかの worktree の `git status` に、コミットしていない変更か ignore されていない追跡していないファイルがある |
  | stash | `refs/stash` がある |
  | 途中の操作 | 主の worktree か、作業領域の中のほかの worktree で、rebase、merge、cherry-pick、revert、bisect のどれかが途中で止まっている |
  | push したブランチ | `origin` の remote-tracking ブランチのうち、reflog に `update by push` があるもの。最後に push したコミットは、その最新の項目のコミット |
  | ローカルのブランチ | 名前、先頭、ほかにないコミット（`origin` の remote-tracking ブランチにも `refs/acp/target` にもないもの）を持つか |
  | detached HEAD | 主の worktree と、作業領域の中のほかの worktree のそれぞれで、ローカルのブランチにもないほかにないコミットを持てば、`HEAD` という名前のローカルのブランチとして加える |

  ほかの worktree（サブエージェントのものなど）は、`.git/worktrees/<名前>` から見つける。記録されたパスは
  エージェントが書けるので、そのまま信じず、その末尾を作業領域の下で探し（`..` やシンボリックリンクで外に出るものは
  使わない）、`--git-dir` と `--work-tree` で読む（[ADR 0042](../../adr/app/0042-judge-linked-worktrees-in-cleanup.md)）。
  末尾は作業領域の下に重ねて探すので、読むコンテナでも、記録のうち作業領域の下の部分（`.claude/worktrees/<名前>`
  など）で見つかる（[ADR 0043](../../adr/app/0043-read-workspaces-in-throwaway-containers.md)）。

- **関係**：`acp workspace-state` が、作業ディレクトリの作業領域についてこれを回し、JSON で出す。control-plane は
  それを、[作業領域を読むコンテナ](containers.md#workspace-reader "在りか")の中で動かし、
  usecase の `SessionWorkspace` として[片付け](#cleaner)に渡す。control-plane のプロセスでは回さない。
  作業領域はエージェントが書け、git はリポジトリの設定が名指すコマンド（`core.fsmonitor`、フィルタなど）を動かすため。
  `acp cleanup-check` は、エージェントのコンテナの中でこれをその場で回す（エージェント自身のコンテナなので、読む
  コンテナは要らない。[ADR 0045](../../adr/app/0045-agents-check-cleanup-with-the-control-planes-binary.md)）。
  `acp cleanup-check` は、作業ディレクトリによらず、control-plane が渡した作業領域（`ACP_WORKSPACE`）を読む。
  作業領域の外に clone したリポジトリ（`/tmp` など）の中で動かされても、そこを判定しない。
- **当てはめた原則・パターン**：[事実と判定を分ける](../../../platform/plugins/principles/docs/reference/patterns.md#facts-and-judgement)：事実だけを返し、
  判定しない（[事実と判定](#facts)）。

<a id="github-branches"></a>

### GitHub のブランチ（`internal/adapter/githubapp/branches.go` の `Branches`）

- **責務**：GitHub のリポジトリで、ブランチの事実を読み、ブランチを消す。聞くリポジトリは、作るときに渡す origin の
  読み方で決まる（片付けはターゲットの `origin`、`acp cleanup-check` は作業領域の `origin`）。
- **置くもの / 置かないもの**：
  - `List`：名前の前方一致（`matching-refs`）でリモートのブランチを探す。作業領域が知らないブランチも見つかる。
  - `Inspect`：ブランチごとに、リモートにあるか、先頭、そのブランチを head ブランチとする PR（開いている、
    マージした、マージせずに閉じたもののすべて。リポジトリの持ち主のブランチのもの）、リモートにあるときだけ
    base ブランチとする開いている PR を読む。問い合わせたコミットと先頭について、既定のブランチ、そのブランチを
    head ブランチとするマージした PR の head コミット、マージせずに閉じた PR の head コミット、そのブランチ自身の
    それぞれに入っているかを compare で確かめる。PR は、head ブランチと base ブランチのそれぞれで 1 ページ
    （100 件）だけ読む。
  - `Remove`：名前を挙げたブランチを消し、消せたものを返す。もうないブランチと保護されたブランチは飛ばす。
  - GitHub App のトークンがないか、`origin` が GitHub でなければ、`List` と `Inspect` は何も返さず、`Remove` は
    何もしない。
- **関係**：usecase の `SessionBranches` を実装する。トークンはエージェントの GitHub App のもの
  （[認証情報](credentials.md "在りか")）。
- **当てはめた原則・パターン**：[層に分けた構成](../../../platform/plugins/principles/docs/reference/patterns.md#layered-architecture)：GitHub の API の
  形はここに閉じ、usecase には `BranchFacts` だけを返す。どのブランチを消すかは決めない（[事実と判定](#facts)）。
