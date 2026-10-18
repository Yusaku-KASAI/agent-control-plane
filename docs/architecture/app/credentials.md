# 認証情報

control-plane が認証情報を発行し、エージェントのコンテナに渡し、動いている間に更新する仕組み。コンテナが何を
受け取るか（環境変数、ファイルの場所、読み取り専用であること）は[ハーネスの契約](../../../platform/docs/reference/harness.md "在りか")
にあり、ここでは control-plane の側を書く。値の用意の仕方は[運用の手順](../../guides/operations.md "在りか")にある。

## 構成

安全はコンテナの隔離と、ロールに合わせて絞った短命の認証情報で守る（[ADR 0003](../../adr/app/0003-isolation-as-the-guardrail.md)、
[最小権限](../../../platform/plugins/principles/docs/reference/principles.md#least-privilege)）。
認証情報は外部サービスごとの発行元が別々に作り、一つにまとめてから使う。

```
ホストの秘密のファイル（ACP_SECRETS_DIR）     acp-state（Slack の更新用トークン）
        │                                        │
        ▼                                        ▼
credentialIssuers ◀── claudeauth / githubapp / slackauth
        │ domain.Credentials
        ├──▶ 作業領域の clone、片付けでのブランチの確認（その場で使う）
        ▼
runner が acp-credentials/<セッション ID>/ に書く ◀── dispatcher が一定間隔で発行し直す
        │ 読み取り専用でマウント（Claude Code の認証だけは環境変数）
        ▼
エージェントのコンテナ
```

- 発行するのは、セッションを作るとき（clone に使う）、コンテナを起動するとき、コンテナが動いている間の
  一定間隔ごと、片付けでセッションのブランチを調べるとき。どれも同じ `CredentialIssuer` を呼ぶ。
- 発行元ごとに、そのエージェントに設定がなければ空の値を返す。空の認証情報はコンテナに渡らず、対応する
  環境変数も設定されない。GitHub App と Slack アプリはエージェントごとに任意で、Claude Code の認証だけは必ずある。
- Slack の Bot トークンは、control-plane の Bot とエージェントが同じものを使う。エージェントは Bot が招待された
  チャンネルを読み書きできる。
- 拡張（プラグイン）は、渡された認証情報の組み合わせを見て動きを変える（[ADR 0006](../../adr/app/0006-external-auth-by-capability.md)）。
  たとえば slack プラグインは `SLACK_BOT_TOKEN_FILE` があるかで使える操作を決める
  （[slack プラグイン](../../../platform/plugins/slack/docs/overview.md "在りか")）。
- ADR 0006 が決めた、拡張が要る機能を宣言して control-plane が方式を割り当てる仕組みは、まだない。拡張が使う外部
  サービス（GitHub、Slack）は、提供方式が一つずつで、エージェントの設定にあればその認証情報を渡す。

<a id="secrets"></a>

### ホストに置く秘密

| もの | 設定 | 読む時 |
| --- | --- | --- |
| Claude Code の OAuth トークン（`claude setup-token`）か API キー | `claude.oauth_token_path` か `claude.api_key_path`（どちらか一つ） | 発行のたび |
| GitHub App の秘密鍵 | `agents[].github.private_key_path` | 起動時 |
| Slack の App-Level Token | `agents[].slack.app_token_path` | 起動時。control-plane の Socket Mode だけに使い、エージェントには渡さない |
| Slack の Bot トークン | `agents[].slack.bot_token_path` | ローテーションなしなら発行のたび。ありなら最初の交換のときだけ |
| Slack の client secret | `agents[].slack.rotation.client_secret_path` | 起動時 |

ファイルはどれも `.env` の `ACP_SECRETS_DIR` の下に置き、control-plane のコンテナにはホストと同じパスで読み取り
専用でマウントされる（[コンテナ](containers.md "在りか")）。項目の意味と置き場所は [acp.example.yaml](../../../acp.example.yaml "在りか") と
[.env.example](../../../.env.example "在りか") が正。

## 構成要素

### `credentialIssuers`（`cmd/acp/serve.go`）

- **責務**：発行元をまとめ、エージェントに渡す認証情報を一つの `domain.Credentials` にする。
- **置くもの / 置かないもの**：発行元を順に呼び、空でない項目を重ねること。どれか一つが失敗したら全体を失敗にする。
  発行の中身は置かない。
- **関係**：usecase の `CredentialIssuer` を実装し、dispatcher と片付け（`SessionCleaner`）に渡す。
  発行元も同じインターフェースを実装する。
- **当てはめた原則・パターン**：[層に分けた構成](../../../platform/plugins/principles/docs/reference/patterns.md#layered-architecture)：usecase は
  発行元の技術を知らず、一つのインターフェースだけを見る。
  [高凝集・疎結合](../../../platform/plugins/principles/docs/reference/principles.md#cohesion)：外部サービスを足すときは発行元を一つ足し、
  ここに項目を一つ足すだけで済む。

### `internal/domain`（`Credentials`）

- **責務**：エージェントに渡しうる認証情報の項目を定める。
- **置くもの / 置かないもの**：GitHub のトークン、Slack の Bot トークン、Claude Code の OAuth トークンか API キー。
  空の値は、その機能がないことを表す。どう渡すかは置かない。
- **当てはめた原則・パターン**：[スキーマを先に](../../../platform/plugins/principles/docs/reference/principles.md#schema-first)：発行元と runner は
  この形だけを受け渡す。

### `internal/adapter/claudeauth`

- **責務**：Claude Code の認証情報を、ホストのファイルから発行のたびに読む。
- **置くもの / 置かないもの**：OAuth トークンか API キーのどちらか一つ（どちらも、どちらもないのは設定の読み込みで
  弾く）。
- **関係**：runner はこれを環境変数 `CLAUDE_CODE_OAUTH_TOKEN` か `ANTHROPIC_API_KEY` で渡す。環境変数はコンテナの
  作成時に決まるので、ファイルを差し替えたら次に起動するコンテナから効く。
- **当てはめた原則・パターン**：[関心の分離](../../../platform/plugins/principles/docs/reference/principles.md#soc)：Claude Code の認証方式は
  この発行元に閉じ、方式を差し替えても他は変わらない（[ADR 0007](../../adr/app/0007-pluggable-claude-auth.md)、
  [0023](../../adr/app/0023-claude-auth-and-per-agent-config.md)）。

### `internal/adapter/githubapp`（`issuer.go`）

- **責務**：エージェントごとの GitHub App のインストールトークンを、発行のたびに新しく作る。
- **置くもの / 置かないもの**：秘密鍵で署名した JWT（RS256、有効期間 10 分未満）で
  `POST /app/installations/<ID>/access_tokens` を呼ぶこと。トークンは 1 時間で失効し、手元には持ち続けない。
  秘密鍵は起動時に読み込む（PKCS#1 か PKCS#8 の RSA）。
- **関係**：エージェントは自分の App のトークンだけを受け取り、できることは App の権限で決まる
  （[ADR 0011](../../adr/app/0011-github-access-via-per-agent-apps.md)）。同じパッケージの `branches.go` は、
  片付けで発行したトークンを使ってリモートのブランチを調べる（[作業領域とブランチ](workspace.md#github-branches "在りか")）。

### `internal/adapter/slackauth`

- **責務**：エージェントごとの Slack アプリの Bot トークンを、今使えるものとして返す。
- **置くもの / 置かないもの**：
  - `Source`：今の Bot トークンを返すインターフェース。`Issuer` はこれを `domain.Credentials` にする。
  - `FileSource`：ローテーションなし。ファイルを呼ばれるたびに読む。
  - `RotatingSource`：トークンローテーションあり（[ADR 0022](../../adr/app/0022-slack-bot-token-rotation.md)）。
    状態がなければ `bot_token_path` の長期のトークンを `oauth.v2.exchange` で交換し、以降は失効の 2 時間前を過ぎたら
    `oauth.v2.access` で更新する。更新用のトークンは一度しか使えないので、受け取ったら使う前に状態のファイル
    （`<state_dir>/slack/<エージェント名>.json`、`acp-state` ボリューム）へ一時ファイルと rename で保存する。
    `Run` が 10 分ごとに確かめ、失敗はログに出して次の回にやり直す。
- **関係**：同じ `Source` を、control-plane の Slack Bot（`slackbot`）とエージェントへの発行の両方が使うので、
  更新したトークンはどちらにも届く。
- **当てはめた原則・パターン**：[単一の情報源](../../../platform/plugins/principles/docs/reference/principles.md#ssot)：今のトークンを持つのは
  エージェントごとに一つの `Source` だけ。

### `acp slack-auth`（`cmd/acp/slackauth.go`）

- **責務**：ローテーションの状態を失ったときに、OAuth の認可コードから Slack のトークンを取り直す。
- **置くもの / 置かないもの**：コードがなければ手順を表示し、あれば `RotatingSource.RedeemCode` で状態のファイルを
  書く。動いている control-plane は状態を読み直さないので、取り直したあとは再起動する。手順は
  [運用の手順](../../guides/operations.md "在りか")にある。
- **関係**：serve と同じ `slackTokenSources` で `RotatingSource` を作るので、状態のファイルの場所は同じになる。

<a id="session-credentials"></a>

### セッションの認証情報のディレクトリ（`internal/adapter/docker/credentials.go`）

- **責務**：発行した認証情報を、セッションごとのディレクトリにファイルとして書き、コンテナに読み取り専用でマウント
  させる。
- **置くもの / 置かないもの**：`<credentials.dir>/<セッション ID>/` の下に、`gh/config.yml` と `gh/hosts.yml`（GitHub
  のトークン）、`slack/bot-token`、`git/co-authors`（今のターンの依頼者。認証情報ではないが、ターンの途中で書き換える
  ので同じ場所に置く）。ファイルは一時ファイルからの rename で置き換え、読む側が書きかけを見ないようにする。
  Claude Code の認証情報は環境変数で渡すので、ここには置かない。
- **関係**：
  - 置き場所は control-plane だけが書ける `acp-credentials` ボリュームで、エージェントのコンテナにはそのセッション
    のサブパスだけをマウントする。ボリュームを使わない設定は [acp.example.yaml](../../../acp.example.yaml "在りか") にある。
  - runner がコンテナを起動する前に書き、動いている間は `Process.UpdateCredentials` で書き換える。
  - `sessions.root` は devcontainer から読めるので、そこには置かない（[ADR 0018](../../adr/app/0018-refreshable-credentials.md)）。
  - セッションを片付けるとき、`SessionFiles` がディレクトリごと消す（[コンテナ](containers.md "在りか")）。

### 動いている間の更新（`internal/usecase/dispatcher.go`）

- **責務**：セッションのコンテナが動いている間、認証情報を一定間隔で発行し直して書き換えさせる。
- **置くもの / 置かないもの**：ワーカーごとのタイマー（`CredentialRefresh`。`serve.go` で 40 分。インストール
  トークンの 1 時間より短い）。コンテナがなければ何もしない。発行か書き換えに失敗したら、そのプロセスを止め、
  実行中のターンを失敗にする。
- **関係**：コンテナの中の `gh` と slack プラグインはファイルを毎回読むので、書き換えはそのまま効く。
  セッションのライフサイクルは[セッション](sessions.md "在りか")にある。
