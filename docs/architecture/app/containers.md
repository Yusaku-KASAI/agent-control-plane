# エージェントのコンテナ

control-plane がエージェントを Docker のコンテナで動かす仕組み。コンテナの中から見た環境（動き方、寿命、渡される
もの）は[ハーネスの契約](../../../platform/docs/reference/harness.md "在りか")に、ターゲットのイメージが満たすことは
[ターゲットの契約](../../../template/docs/reference/contract.md "在りか")にあり、ここでは control-plane の側を書く。
起動の手順は[運用の手順](../../guides/operations.md "在りか")にある。

## 構成

```
ホスト（Docker）
├─ control-plane（compose の control-plane サービス、acp serve）
│    │ DOCKER_HOST=tcp://docker-proxy:2375（内部ネットワーク）
│    ▼
├─ docker-proxy（Docker ソケットを持つ唯一のコンテナ。許可した API だけを通す）
│    │
│    ▼
├─ エージェントのコンテナ（動いているセッションごとに 1 つ。ターゲットのイメージ）
│    claude -p --input-format stream-json ...（標準入出力は control-plane が attach）
│    マウント：作業領域 / エージェントの Claude Code の設定 / セッションの認証情報（読み取り専用）
│             / ターゲットの隣のマーケットプレイス（読み取り専用）
└─ 作業領域を読むコンテナ（片付けで判定するたびに 1 つ、使い捨て。ターゲットのイメージ、ネットワークなし）
     acp workspace-state（control-plane のバイナリ）
     マウント：作業領域（読み取り専用）/ control-plane のバイナリ（読み取り専用）
```

- control-plane は常駐するサーバーとしてコンテナで動き、`acp run` などの CLI はその HTTP API のクライアントになる
  （[ADR 0014](../../adr/app/0014-control-plane-as-a-server.md)）。
- Docker を操作するのは control-plane だけで、エージェントのコンテナには Docker ソケットを渡さない
  （[ADR 0004](../../adr/app/0004-docker-access-only-via-control-plane.md)）。control-plane もソケットを持たず、プロキシ
  越しに操作する（[ADR 0015](../../adr/app/0015-docker-api-allowlist-proxy.md)）。
- 動いているセッションごとに、1 つのコンテナで Claude Code を動かし続け、依頼を標準入力に送る
  （[ADR 0017](../../adr/app/0017-long-lived-session-process.md)）。いつコンテナを起動し、止めて消すかは
  usecase の dispatcher が決め、runner はその指示でコンテナを操作する。段階ごとに起きることは
  [セッション](sessions.md "在りか")にある。
- control-plane とエージェントのコンテナは、どちらもホストのパスで bind する。設定に書くパス（ターゲット、
  `sessions.root`、`usage_log`、秘密のファイル）は、control-plane のコンテナにもホストと同じパスでマウントする。エージェントの
  コンテナのマウントはホストの Docker が解決するので、パスを読み替えずに済む。

## 構成要素

<a id="service"></a>

### `compose.yaml` の `control-plane` サービスと `Dockerfile`

- **責務**：control-plane（`acp serve`）を常駐するコンテナとして動かす。
- **置くもの / 置かないもの**：
  - イメージは `Dockerfile` で、`acp` と git を入れ、名前付きボリュームのマウント先を作っておく。
  - `ACP_UID`、`ACP_GID` のユーザーで動き、作業領域を作る。エージェントのコンテナはイメージのユーザーで作業領域に
    書くので、この値をイメージのユーザーと同じにして、作業領域の持ち主をそろえる（値は [.env.example](../../../.env.example "在りか")）。
  - マウント：設定ファイル（読み取り専用）。`ACP_HOST_ROOT`（読み書き可）と `ACP_SECRETS_DIR`（読み取り専用）を、
    ホストと同じパスに（置き場所は [.env.example](../../../.env.example "在りか")）。
    名前付きボリュームの `acp-credentials`（セッションの認証情報）、`acp-claude`（エージェントの Claude Code の設定）、
    `acp-state`（control-plane だけが読み書きする状態）、`acp-tools`（control-plane が起動のたびに自分のバイナリを
    置き、作業領域を読むコンテナが使う）。名前は固定する（`acp-credentials`、`acp-claude`、`acp-tools` は、
    control-plane が作るコンテナから名前で指す）。
  - API は、ホストの `127.0.0.1:8787` で届く。ネットワークは compose のプロジェクトの既定のもの
    （`agent-control-plane_default`）で、devcontainer もエージェントのコンテナも入らない（[ターゲットの契約](../../../template/docs/reference/contract.md#control-plane "在りか")）。
    リクエストには `ACP_API_TOKEN` を要求する。
  - 止めるときの猶予は 1 分で、プロセスを止める期限（`process.go` の `Close`）より長い。実行中のターンを中断して
    スレッドに知らせ終えるまで待つ（[依頼と知らせ](requests.md#shutdown "在りか")、[ADR 0024](../../adr/app/0024-restart-abandons-running-work.md)）。
- **関係**：プロキシにだけ、内部ネットワーク `docker-proxy` でつながる。秘密のファイルとボリュームの中身は
  [認証情報](credentials.md "在りか")にある。

### `compose.yaml` の `docker-proxy` サービス

- **責務**：Docker ソケットを持つ唯一のコンテナとして、control-plane が使う Docker API だけを通す。
- **置くもの / 置かないもの**：`wollomatic/socket-proxy` に、HTTP メソッドごとのパスの正規表現で許可を書く。
  許可は API ごとに 1 行にする。通すのは、`_ping`（HEAD と、クライアントが代わりに使う GET）、コンテナの一覧、作成、
  attach、起動、終了待ち、削除だけ。許可は静的な正規表現で、プロキシはパス（クエリを除く）だけを照らす。コンテナ
  ID は実行時に決まるので、どのコンテナへの操作かは絞れない。接続を受けるのは `control-plane` からだけで、プロキシ自身は読み取り専用のファイルシステムで、capability をすべて落として動く。
  作成するコンテナの中身（マウント、イメージ、特権）は検査しない。
- **関係**：許可の一覧にない API はプロキシが拒む。runner が使う Docker API（`runner.go` の `dockerAPI`）とこの一覧が
  一致していることは、`proxy_test.go` が確かめる。本物の Docker のクライアントで `dockerAPI` のメソッドをすべて呼び、
  送られた要求がどれも許可に当たり、どの許可もどれかの要求に当たることを、socket-proxy と同じ照らし方で見る。
  許可の正規表現に選択（`|`）があるときと、使わない操作の見本（exec、コンテナへの書き込みと読み出し、イメージ、
  ビルドなど）がどれかの許可に当たるときも落ちる。socket-proxy は、`-allow<METHOD>=<正規表現>` のほかにも許可を読む
  （引数を別の要素に分けた書き方、`SP_ALLOW_` の環境変数、`-proxycontainername` を付けたときの呼ぶ側のコンテナの
  ラベル）。試験はそれらを読まないので、知らない書き方は拒む。サービスのキーが今使っているもの（`image`、`command`、
  `volumes` など）のほかにあるときと、引数が `-listenip=`、`-allowfrom=`、`-allow<METHOD>=` のほかにあるときも落ちる。
  Compose が `compose.yaml` に重ねる override のファイル（`compose.override.yaml` など）があるときも落ちる。
- **当てはめた原則・パターン**：[仕組みで守る](../../../platform/plugins/principles/docs/reference/principles.md#mechanisms)：control-plane が
  できる Docker の操作を、コードの約束ではなくプロキシの許可で絞る。許可と runner の呼び出しのずれも、人が読むのでは
  なく試験で見つける。
  [最小権限](../../../platform/plugins/principles/docs/reference/principles.md#least-privilege)、[拒否を既定にする](../../../platform/plugins/principles/docs/reference/principles.md#fail-safe-defaults)：通すのは使う API だけで、
  一覧にないものはすべて拒む。

<a id="runner"></a>

### `internal/adapter/docker`（`runner.go`）

- **責務**：usecase の `Runner` を実装し、セッションのエージェントのコンテナを作って起動する。
- **置くもの / 置かないもの**：
  - `Start`：宣言されたプラグインを読み、セッションの認証情報を書き、エージェントの Claude Code の設定
    ディレクトリを作ってから、コンテナを作り、attach し、起動する。途中で失敗したらコンテナを消す。
  - `containerSpec`：コンテナの形を一か所で決める。
    - イメージはエージェントのターゲットのイメージ。ユーザーは指定せず、イメージのユーザーで動く。
    - コマンドは `claude -p` を `stream-json` の入出力、`--agent <ロール>`、`--session-id` か `--resume`
      （終えたターンが 0 かで決まる）、`--dangerously-skip-permissions`、プラグインごとの `--plugin-dir` で動かす。
    - 作業ディレクトリは `/workspace/<ターゲットのディレクトリ名>`。Claude Code は会話の履歴を作業ディレクトリで
      引くので、セッションの間変えない。
    - 環境変数は、`CLAUDE_CONFIG_DIR`、`ACP_SESSION_BRANCH`、`ACP_CO_AUTHORS_FILE` と、認証情報とコミットの作者
      から決まるもの。
    - マウントは、作業領域（書き込める）、エージェントの Claude Code の設定、セッションの認証情報（読み取り専用）、
      ターゲットの隣のマーケットプレイス（読み取り専用）。
    - ラベル `acp.session`（セッション ID）と `acp.agent`（エージェント名）。Docker Compose がプロジェクトのコンテナを探す
      ラベル（`com.docker.compose.project`、`com.docker.compose.service`）は空にする。`docker compose build` で作った
      イメージはこれを持ち、コンテナはイメージのラベルを引き継ぐので、空にしないとエージェントのコンテナが devcontainer の
      compose のプロジェクトに数えられ、VS Code の Reopen in Container や `docker compose down` の対象になる。
    - 標準入力を開いておき、閉じるとプロセスが終わる。
  - `RemoveLeftovers`：ラベル `acp.session` の付いたコンテナを、動いているものも止まっているものもすべて消し、
    そのセッションを返す。ラベル `acp.workspace-state` の付いた、作業領域を読むコンテナの残りも消す。いつ呼ばれるかは[依頼と知らせ](requests.md#shutdown "在りか")にある。
  - いつ起動し、いつ止めるか（休ませる時間、停止）は置かない。dispatcher が決める。
- **関係**：usecase の `Runner` と `StartRequest` だけを見る。イメージへの前提はターゲットの契約にある。
  エージェントに渡すものの一覧はハーネスの契約と一致させる。
- **当てはめた原則・パターン**：[層に分けた構成](../../../platform/plugins/principles/docs/reference/patterns.md#layered-architecture)：判定は usecase に
  置き、ここは Docker の操作と事実（プロセスの出力）だけを扱う。
  [単一の情報源](../../../platform/plugins/principles/docs/reference/principles.md#ssot)：コンテナの形は `containerSpec` にだけある。
  [最小権限](../../../platform/plugins/principles/docs/reference/principles.md#least-privilege)：エージェントのコンテナには Docker ソケットも特権も渡さない。渡す認証情報は
  [認証情報](credentials.md "在りか")にある。

### `internal/adapter/docker`（`process.go`）

- **責務**：動いている 1 つのコンテナと、Claude Code の `stream-json` でやり取りする。
- **置くもの / 置かないもの**：
  - 送る：依頼（ID 付きの `user` メッセージ）、`control_request` の `interrupt`（取り込まれていない依頼も捨てる）と
    `stop_task`。
  - 読む：標準出力の行を、ターンの始まり（`system/init`）、終わり（`result`）、バックグラウンドの処理の変化、依頼の
    取り込み（`--replay-user-messages` で返る依頼）、今していること（最後のツール呼び出しかサブエージェントの
    テキスト）とエージェントの発言（エージェント本体のテキスト）の出来事にする。
    ターンの終わりは、`subtype` が `error_during_execution` で、`interrupt` を送ってから最初の `result` か、
    `terminal_reason` が `aborted_streaming` か `aborted_tools` なら中断とする
    （[ADR 0033](../../adr/app/0033-stopped-turns-by-terminal-reason.md)）。ほかに `is_error` が立つか `subtype` が
    `error` で始まればエラーとし、`result` の文面がなければ、`errors` から診断の行を除いたものを文面にする。
    エラーのうち、`api_error` が `usage_limit_reached` のもの、`api_error` がなく最後に受けた `rate_limit_event` が
    上限に達していることを示している（`rejected` で、追加の利用分を使っておらず、戻る時刻の前）ものを、使用上限に
    よるものとする（[ADR 0029](../../adr/app/0029-turns-ending-on-errors.md)）。
    標準エラーは最後の 8 KiB を持ち、異常終了の理由に添える。
  - `UpdateCredentials` と `SetCoAuthors`：セッションの認証情報のディレクトリのファイルを書き換える。
  - `Close`：標準入力を閉じ、終わるか期限（dispatcher の `StopTimeout`、30 秒）が来るまで待ってから、コンテナを
    強制的に消す。
- **関係**：usecase の `Process` を実装する。出来事をどう扱うか（受信箱、状態の表示、休ませること）は dispatcher にある。
- **当てはめた原則・パターン**：[関心の分離](../../../platform/plugins/principles/docs/reference/principles.md#soc)：Claude Code の出力の形を
  知るのはここだけで、usecase は `ProcessEvent` だけを見る。

### `internal/adapter/docker`（`plugins.go`）

- **責務**：ターゲットが宣言したプラグインのうち、control-plane が読み込ませるものと、その場所を決める。
- **置くもの / 置かないもの**：どのプラグインをどこから読み込ませるかの決まりは、ターゲットの契約の
  [プラグインとロール](../../../template/docs/reference/contract.md#plugins "在りか")にある。ここはそれを実装する。
  - 作業領域の `.claude/settings.json` の `enabledPlugins` と `extraKnownMarketplaces` を読み、`directory` の
    マーケットプレイスのプラグインを選んで `--plugin-dir` で渡す。ヘッドレスの Claude Code は宣言されたプラグインを
    入れないため。
  - マーケットプレイスのパスは、契約の形（devcontainer でのパス）のほか、リポジトリからの相対パスも受け付ける。
    ターゲット自身の中か隣のリポジトリかの分け方と、それぞれをどう見せるかは契約のとおり。
  - プラグインの場所はマーケットプレイスの `.claude-plugin/marketplace.json` で引き、相対パスの source だけを扱う。
  - 作業領域はエージェントが書けるので、これらのファイルは control-plane のプロセスで、読む元のディレクトリ（作業領域、
    ホストのターゲットの checkout、隣のマーケットプレイス）の外を読まないように読む（`readUntrusted`）。シンボリックリンクは、その中に留まるものだけを
    たどる。待たずに開いて、開いたものが通常のファイルかを確かめ（名前付きパイプ、デバイスは断る）、1 ファイル 1 MiB、
    一つのコンテナのために合計 4 MiB までしか読まない。断ったらコンテナを起こさない（[ADR 0044](../../adr/app/0044-read-workspace-files-confined.md)）。
  - ターゲット自身の中のマーケットプレイスは作業領域の設定の宣言から、隣のマーケットプレイス（マウントするもの）は
    ホストのターゲットの checkout の設定の宣言からだけ決める。作業領域の設定とリンクはエージェントが書けるので、
    ほかのセッションの作業領域や外のディレクトリをマウントさせないため。
- **関係**：決まりの理由は [target-template ADR 0004](../../../template/docs/adr/0004-plugins-declared-in-settings.md)、
  [0007](../../../template/docs/adr/0007-marketplaces-beside-the-target.md) にある。
- **当てはめた原則・パターン**：[使う側に届く契約](../../../platform/plugins/principles/docs/reference/patterns.md#published-language)：
  プラグインの宣言はターゲットの契約で、control-plane はそれを読んで動かすだけ
  （[ADR 0001](../../adr/app/0001-repository-centric-agents.md)、[0005](../../adr/app/0005-extensions-standard-first.md)）。

<a id="claude-config"></a>

### エージェントの Claude Code の設定ディレクトリ

- **責務**：エージェントごとに、会話の履歴と auto memory を置く Claude Code の設定ディレクトリを用意する。
- **置くもの / 置かないもの**：`acp-claude` ボリューム（`claude.config_dir`）の `<エージェント名>/` を、コンテナの
  `/home/node/.claude` にマウントし、`CLAUDE_CONFIG_DIR` で指す。ボリュームを使わない設定は
  [acp.example.yaml](../../../acp.example.yaml "在りか") にある。同じエージェントのセッションは、同時に動くものも含めてこれを共有する。
  devcontainer の Claude Code の設定はエージェントに渡さず、エージェントの間と devcontainer とで会話の履歴と auto memory を分ける
  （[ADR 0023](../../adr/app/0023-claude-auth-and-per-agent-config.md)）。
- **関係**：Claude Code の認証は設定ディレクトリではなく、発行した認証情報で行う（[認証情報](credentials.md "在りか")）。

<a id="workspace-reader"></a>

### `internal/adapter/docker`（`workspace.go`）

- **責務**：usecase の `SessionWorkspace` を実装し、セッションの作業領域が持っているもの（[作業領域の状態](workspace.md#state "在りか")）を、
  control-plane のプロセスの外で読む。作業領域はエージェントが書け、git はリポジトリの設定が名指すコマンド
  （`core.fsmonitor`、フィルタなど）を動かし、名前付きパイプなどで止まりもするため（[ADR 0043](../../adr/app/0043-read-workspaces-in-throwaway-containers.md)）。
- **置くもの / 置かないもの**：
  - `InstallTool`：`acp serve` の起動のたびに、動いている自分のバイナリを `tools.dir`（ボリュームは `tools.volume`）に
    置き換える。
  - `State`：作業領域が git のリポジトリなら、使い捨てのコンテナで `acp workspace-state` を動かし、標準出力の JSON を
    読む。期限（2 分）を過ぎたら諦めてエラーにし、どの終わり方でもコンテナを消す。読む出力は標準出力 4 MiB、
    標準エラー 16 KiB まで。返る事実はエージェントが左右できるので、ブランチの名前、コミット、途中の操作が git の
    書きうる形かを確かめ、外れたらエラーにする。
  - `stateSpec`：そのコンテナの形を一か所で決める。
    - イメージはエージェントのターゲットのイメージ（git があることはターゲットの契約）。エントリポイントは
      `/run/acp/bin/acp`。ユーザーはイメージのユーザー。
    - 作業ディレクトリと作業領域のマウント先は、エージェントのコンテナと同じ `/workspace/<ターゲットのディレクトリ名>`。
      git がエージェントのコンテナで記録したパス（worktree など）が、そのまま開ける。
    - マウントは、作業領域（読み取り専用）と、`tools` のバイナリ（`/run/acp/bin`、読み取り専用）だけ。認証情報も
      環境変数も渡さない。
    - ネットワークなし、ケーパビリティなし、`no-new-privileges`、読み取り専用のルート（`/tmp` だけ 16 MiB の tmpfs）。
      プロセス数とメモリにも上限を付ける。エージェントが設定に書いたコマンドが、依頼と関係なく動くため。
    - ラベル `acp.workspace-state`。Docker Compose のラベルは、エージェントのコンテナと同じく空にする。
- **関係**：片付け（[片付け](sessions.md#cleaner "在りか")）から呼ばれる。使う Docker の API はエージェントのコンテナと同じで、
  プロキシの許可は変わらない。
- **当てはめた原則・パターン**：[最小権限](../../../platform/plugins/principles/docs/reference/principles.md#least-privilege)：エージェントが書いたものを読む
  git には、作業領域を読む権限のほか何も渡さない。

### `internal/adapter/docker`（`sessionfiles.go`）

- **責務**：セッションを片付けるとき、runner が作業領域の外に残したものを消す。
- **置くもの / 置かないもの**：セッションの認証情報のディレクトリと、エージェントの Claude Code の設定ディレクトリの
  1〜2 段下にあるセッション ID で始まる名前のもの（`projects/<ディレクトリ>/<ID>.jsonl` など）。ID は UUID なので
  ほかのセッションのものと重ならず、Claude Code のファイルの配置を知らずに済む。記録と作業領域は消さない
  （`SessionRepository` が消す）。
- **関係**：usecase の `SessionFiles` を実装し、[片付け](sessions.md#cleaner "在りか")から
  呼ばれる。runner と同じ `Options` で場所を知る。
- **当てはめた原則・パターン**：[高凝集・疎結合](../../../platform/plugins/principles/docs/reference/principles.md#cohesion)：runner が作るものと
  消すものを同じパッケージに置く。
