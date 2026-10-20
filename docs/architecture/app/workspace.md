# 作業領域とブランチ

セッションごとの作業領域の作り方と、セッションのブランチ、片付けのために作業領域と GitHub から読む事実。
作業領域とセッションのブランチの意味は[用語集](../../../platform/docs/reference/glossary.md)、エージェントから
見た作業領域とブランチ、コミットの決まりは[ハーネスの契約](../../../platform/docs/reference/harness.md "在りか")にある。
セッション全体は[セッション](sessions.md "在りか")、作業領域をマウントするコンテナは[コンテナ](containers.md "在りか")、
GitHub App のトークンは[認証情報](credentials.md "在りか")にある。

## 構成

```
ターゲットのリポジトリ（ホスト）
  │ clone
  ▼
作業領域 <sessions.root>/<ID>/workspace/<ターゲットのディレクトリ名>
  origin            = ターゲットの origin の URL
  refs/acp/target   = clone した時点のターゲットの HEAD
  acp/<ID の先頭8文字> = リモートの既定のブランチ（なければターゲットの HEAD）から切ったブランチ
  .git/hooks/commit-msg = 依頼者を Co-authored-by に足す
  .git/hooks/pre-push   = push の前に、新しいブランチの名前を確かめ、取り込んでいないリモートの先頭の上書きを断り、make check を通す
  │ push（エージェント）
  ▼
GitHub のブランチと PR
```

- 作業領域は、セッションを作るときに一度だけ作り、片付けまで消さない（[ADR 0010](../../adr/app/0010-per-session-cloned-workspace.md)）。
  コンテナにはこの作業領域だけをマウントする。
- エージェントはセッションのブランチで作業し、push して PR を作る。作業するブランチの扱い（既存の PR を続ける、別の PR に分ける、ほかの PR に重ねる）は
  [ハーネスの契約](../../../platform/docs/reference/harness.md "在りか")にある（[ADR 0012](../../adr/app/0012-agent-git-workflow.md)）。
- 片付けは、作業領域と GitHub から事実を読み、[片付け](sessions.md#cleaner "在りか")が判定する。作業領域を作るときに
  残す `refs/acp/target` と reflog の設定は、その事実を読むためのもの
  （[ADR 0026](../../adr/app/0026-clean-up-unused-sessions.md)）。

<a id="facts"></a>

### 事実と判定

作業領域と GitHub を読むアダプターは、事実（どのブランチに push したか、どのコミットがどこに入っているか）だけを
返す。残すか消すかは[片付け](sessions.md#cleaner "在りか")の 1 か所が決める。アダプターの側では、どのブランチが
セッションのものかも、どのコミットが失われるかも決めない。

## 構成要素

<a id="prepare"></a>

### 作業領域を作る（`internal/adapter/gitworkspace/provider.go` の `Prepare`）

- **責務**：ターゲットから、セッションの作業領域を作る。
- **置くもの / 置かないもの**：次の順で行う。
  1. ターゲットを `<sessions.root>/<ID>/workspace/<ターゲットのディレクトリ名>` に clone する。
  2. ターゲットに `origin` があれば、作業領域の `origin` をその URL に付け替える（ターゲットのパスはコンテナから
     見えない）。
  3. clone した時点の HEAD を `refs/acp/target` に残し、remote-tracking ブランチの reflog を消えないようにする
     （`gc.refs/remotes/*.reflogExpire` と `reflogExpireUnreachable` を `never`）。
  4. `origin` があり GitHub App のトークンがあれば、`origin` を fetch して既定のブランチ（`origin/HEAD`）を決め、
     そこからセッションのブランチを切る。なければ clone した HEAD から切る。どちらも upstream は設定しない
     （[ADR 0013](../../adr/app/0013-session-branch-from-remote-default.md)）。
  5. [commit-msg フック](#commit-msg)と[pre-push フック](#pre-push)を置く。
- **関係**：usecase の `WorkspaceProvider` を実装する。呼ぶのは、セッションを作るときの `Dispatcher`
  （ロックを持ったまま）。トークンは[認証情報](credentials.md "在りか")の発行したもの。
- **当てはめた原則・パターン**：[層に分けた構成](../../../platform/plugins/principles/docs/reference/patterns.md#layered-architecture)：git の操作は
  ここに閉じ、usecase は作業領域のパスだけを受け取る。

<a id="git"></a>

### git の実行（`internal/adapter/gitworkspace/provider.go` の `gitRunner`）

- **責務**：control-plane が作業領域に対して行う git の操作を、動かす人の環境によらず同じにする。
- **置くもの / 置かないもの**：ホストのユーザーとシステムの git 設定を読まず、端末で聞かない。認証には
  control-plane が発行したトークンだけを使う（認証ヘルパーで渡す）。github.com の SSH の URL は HTTPS に読み替える。
  所有者の違う作業領域も扱えるよう `safe.directory` を `*` にする。状態を読むときに index を書き換えない
  （`GIT_OPTIONAL_LOCKS=0`）ので、エージェントが使っている最中に読んでもぶつからない。
- **当てはめた原則・パターン**：[仕組みで守る](../../../platform/plugins/principles/docs/reference/principles.md#mechanisms)：ホストの鍵や設定を
  使わないことを、呼び出しごとの環境変数で強制する。

<a id="branch"></a>

### セッションのブランチ（`internal/domain/session.go` の `SessionID.Branch`、`OwnsBranch`）

- **責務**：セッションのブランチの名前と、あるブランチをセッションが作ったかを決める。
- **置くもの / 置かないもの**：名前は `acp/<ID の先頭8文字>`。セッションが作ったブランチは、その名前か、それに
  `-` で始まる名前を続けたもの。セッションはほかのブランチ（別のセッションの PR のブランチなど）にも push する
  ことがあり、それは名前ではなく[作業領域の状態](#state)の push の記録で分かる。
- **関係**：名前は `ACP_SESSION_BRANCH` でエージェントに渡す（[コンテナ](containers.md "在りか")）。片付けは、セッションが
  作ったブランチだけをリモートから消す。[pre-push フック](#pre-push)は、セッションが作ったブランチでない名前の
  新しいブランチに注意を出す。
- **当てはめた原則・パターン**：[SSOT](../../../platform/plugins/principles/docs/reference/principles.md#ssot)：名前の決まりはここだけに置き、
  作るとき、渡すとき、片付けるときは同じ関数を使う。シェルで動く pre-push フックだけは、`-` で続ける決まりを写して
  持つ。写しが `OwnsBranch` と食い違わないことは、フックの試験が `OwnsBranch` から期待を求めて確かめる
  （[契約の試験](../../../platform/plugins/principles/docs/reference/patterns.md#contract-test)）。

<a id="commit-msg"></a>

### co-author の commit-msg フック（`internal/adapter/gitworkspace/provider.go` の `commitMsgHook`）

- **責務**：今のターンの依頼者を、エージェントのコミットの `Co-authored-by` に足す（[ADR 0021](../../adr/app/0021-credit-requesters-as-co-authors.md)）。
- **置くもの / 置かないもの**：フックは `ACP_CO_AUTHORS_FILE` の一覧を 1 行ずつ読み、同じ行がなければトレーラーを
  足す。作業領域の `.git/hooks/` にだけ置くので、コミットされない。一覧を書くのは control-plane で、ターンに
  取り込まれた依頼の依頼者を、設定にある git の名前とメールアドレスで書く。
- **関係**：一覧のファイルは認証情報のディレクトリにある（[認証情報](credentials.md "在りか")）。書き換えるのは
  [ワーカー](sessions.md#worker "在りか")が依頼をターンに取り込んだとき。
- **当てはめた原則・パターン**：[仕組みで守る](../../../platform/plugins/principles/docs/reference/principles.md#mechanisms)：エージェントへの
  指示ではなくフックで足す。

<a id="pre-push"></a>

### pre-push フック（`internal/adapter/gitworkspace/provider.go` の `prePushHook`）

- **責務**：push の前に、リモートに新しく作るブランチの名前を確かめ
  （[ADR 0040](../../adr/app/0040-warn-of-new-branch-names-before-push.md)）、取り込んだことのないリモートの先頭を
  上書きする push を断り（[ADR 0041](../../adr/app/0041-refuse-overwriting-remote-tips-never-taken.md)）、ターゲットの
  検査（`make check`）を通す（[ADR 0032](../../adr/app/0032-check-before-push-in-workspace.md)）。
- **置くもの / 置かないもの**：リモートにまだないブランチを作る push で、その名前が
  [セッションが作ったブランチ](#branch)の名前でなければ、注意を出す。push は断らない。セッションのブランチの名前は、
  作業領域を作るときにフックに埋める。リモートにある ref を fast-forward でなく上書きする push で、リモートの
  先頭が、push する手元のブランチ（`HEAD` ならそれが指すブランチ）の reflog のどの項目からもたどれなければ、push を
  断る（git の `--force-if-includes` と同じ確かめを、push のフラグによらず行う）。そのあと、push する ref（消すものは除く）がどれも今の HEAD の
  コミットで、`git status` にコミットしていない変更も ignore されていない追跡していない
  ファイルもないときにだけ、作業ツリーの一番上で `make check` を回す。どれかを満たさないか、検査が落ちれば push を
  断る。Makefile がなければ、名前の注意と上書きの確かめのほかは何も確かめない。検査の前に、git がフックに渡すリポジトリの場所の
  変数（`GIT_DIR` など）を外し、`GIT_CONFIG_*` の設定は残す。作業領域の `.git/hooks/` にだけ置くので、コミット
  されない。
- **関係**：検査のコマンドは[ターゲットの契約の検査](../../../template/docs/reference/contract.md#checks "在りか")。
  エージェントから見た振る舞いは[ハーネスの契約](../../../platform/docs/reference/harness.md "在りか")にある。
- **当てはめた原則・パターン**：[仕組みで守る](../../../platform/plugins/principles/docs/reference/principles.md#mechanisms)：push の前に検査を
  通すことと、新しいブランチの名前に気づかせることを、決まりだけでなく git のフックで守る。

<a id="state"></a>

### 作業領域の状態（`internal/adapter/gitworkspace/provider.go` の `State`）

- **責務**：作業領域が、リモートにないものとして何を持っているかを読む。
- **置くもの / 置かないもの**：次の事実を返す。作業領域がないか git のリポジトリでなければ、何も返さない。

  | 事実 | 読み方 |
  | --- | --- |
  | 作業ツリーの変更 | `git status` に、コミットしていない変更か ignore されていない追跡していないファイルがある |
  | stash | `refs/stash` がある |
  | 途中の操作 | rebase、merge、cherry-pick、revert、bisect のどれかが途中で止まっている |
  | push したブランチ | `origin` の remote-tracking ブランチのうち、reflog に `update by push` があるもの。最後に push したコミットは、その最新の項目のコミット |
  | ローカルのブランチ | 名前、先頭、ほかにないコミット（`origin` の remote-tracking ブランチにも `refs/acp/target` にもないもの）を持つか |
  | detached HEAD | ローカルのブランチにもないほかにないコミットを持てば、`HEAD` という名前のローカルのブランチとして加える |

- **関係**：usecase の `SessionWorkspace` を実装する。読むのは[片付け](sessions.md#cleaner "在りか")。
- **当てはめた原則・パターン**：[事実と判定を分ける](../../../platform/plugins/principles/docs/reference/patterns.md#facts-and-judgement)：事実だけを返し、
  判定しない（[事実と判定](#facts)）。

<a id="github-branches"></a>

### GitHub のブランチ（`internal/adapter/githubapp/branches.go` の `Branches`）

- **責務**：ターゲットの `origin` の GitHub のリポジトリで、ブランチの事実を読み、ブランチを消す。
- **置くもの / 置かないもの**：
  - `List`：名前の前方一致（`matching-refs`）でリモートのブランチを探す。作業領域が知らないブランチも見つかる。
  - `Inspect`：ブランチごとに、リモートにあるか、先頭、そのブランチを head ブランチとする開いている PR、
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
