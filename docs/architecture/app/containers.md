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
└─ エージェントのコンテナ（動いているセッションごとに 1 つ。ターゲットのイメージ）
     claude -p --input-format stream-json ...（標準入出力は control-plane が attach）
     マウント：作業領域 / エージェントの Claude Code の設定 / セッションの認証情報（読み取り専用）
              / ターゲットの隣のマーケットプレイス（読み取り専用）
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
  `sessions.root`、秘密のファイル）は、control-plane のコンテナにもホストと同じパスでマウントする。エージェントの
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
    `acp-state`（control-plane だけが読み書きする状態）。名前は固定する（`acp-credentials` と `acp-claude` は、
    エージェントのコンテナから名前で指す）。
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
  通すのは、`_ping` と `version`、コンテナの一覧、作成、attach、起動、終了待ち、削除だけ。接続を受けるのは
  `control-plane` からだけで、プロキシ自身は読み取り専用のファイルシステムで、capability をすべて落として動く。
  作成するコンテナの中身（マウント、イメージ、特権）は検査しない。
- **関係**：許可の一覧にない API はプロキシが拒むので、runner が使う Docker API とこの一覧は一致している。
- **当てはめた原則・パターン**：[仕組みで守る](../../../platform/plugins/principles/docs/reference/principles.md#mechanisms)：control-plane が
  できる Docker の操作を、コードの約束ではなくプロキシの許可で絞る。
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
    そのセッションを返す。いつ呼ばれるかは[依頼と知らせ](requests.md#shutdown "在りか")にある。
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
    取り込み（`--replay-user-messages` で返る依頼）、今していること（最後のツール呼び出しかテキスト）の出来事にする。
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
