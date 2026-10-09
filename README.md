# agent-control-plane

Claude Code エージェントをコンテナで隔離して実行する制御基盤。

狙い、考え方、全体の構成と目指す姿は [docs/overview.md](docs/overview.md) にある。

## 構成

- `acp serve`：依頼を Slack と HTTP API で受け、エージェントのコンテナを動かすサーバー。`compose.yaml` でコンテナとして
  動かす。Docker へは、使う API だけを通すプロキシ経由でアクセスする。
- `acp run`：サーバーに依頼して結果を表示するクライアント。

## 依頼する

### Slack から

エージェントの Bot をメンションすると、そのスレッドで依頼を受け付け、結果をスレッドに返信する。

- スレッドの中で続けてメンションすると、同じセッションの続きになる。実行中のターンがあれば、終わるまで待つ。
- 本文を `:zap:` で始めると、実行中のターンに流し込む。エージェントがそのターンに取り込んだら、答えは前の依頼への
  返信にまとめられ、流し込んだメッセージには `:white_check_mark:` が付く。
- ターンが始まるとスレッドに状態メッセージが出て、経過時間や今の作業を書き換えながら示す。
- 状態メッセージに `:no_entry_sign:` を付けると、実行中のターン、バックグラウンドの処理、待っている依頼を
  すべて止める。
- `acp.yaml` の `people` に載っている人の依頼でできたコミットには、その人が `Co-authored-by` として付く。

### CLI から

```bash
acp run --agent <name> [--session <id>] [--inject] [--server <url>] "<指示>"
acp stop --agent <name> --session <id> [--server <url>]
```

- サーバーの URL は `--server`、`ACP_SERVER`、`http://127.0.0.1:8787` の順で決まる。API トークンは
  `ACP_API_TOKEN` から読む。
- `--session` を省くと新しいセッションを始め、`--session <id>` を付けるとそのセッションの続きを依頼する。
  結果を stdout に、セッション ID と作業領域のパスを stderr に表示する。
- `--inject` は実行中のターンに指示を流し込む。`acp stop` はそのセッションの実行中のターン、バックグラウンドの
  処理、待っている依頼をすべて止める。

### セッションの動き

- 新しいセッションでは、ターゲットのリポジトリを `<sessions.root>/<セッションID>/workspace/<リポジトリ名>` に
  `git clone` する。`acp/<セッションIDの先頭8文字>` ブランチで作業し、`origin` は元のリポジトリと同じリモートを
  指す。GitHub App があればリモートのデフォルトブランチから、なければ元のリポジトリの HEAD からブランチを切る。
- セッションごとにターゲットのイメージからコンテナを作り、エージェントのロールで Claude Code を動かし続ける。
  依頼はそのプロセスに送り、実行中に届いた依頼は前のターンが終わるまで待たせる。
- 依頼が `sessions.idle_timeout`（既定 15 分）来なければコンテナを片付け、次の依頼で会話を引き継いで起動し直す。
- GitHub App のトークンは、コンテナが動いている間 control-plane が失効前に更新する。
- 使われないまま `sessions.retention`（既定 30 日）を過ぎたセッションは、作業領域、会話の履歴、GitHub の
  ブランチごと消え、スレッドにそう知らせる。消えたあとに同じスレッドで依頼すると、新しいセッションとして始まる。
  開いている PR、セッションが作ったブランチで PR も既定のブランチへの取り込みもないもの、push していない
  コミットがあれば残し、スレッドで片付けを促す。
- control-plane が再起動すると、動いていた処理は中断され、スレッドにそう知らせる。続けるには、もう一度依頼する
  （会話は引き継がれる）。

## サーバーを動かす

設定は [acp.example.yaml](acp.example.yaml)（各項目の意味）と [.env.example](.env.example) をもとに、`acp.yaml` と
`.env` を作る。どちらも gitignore の対象。以下は値の用意の仕方。

### 1. 全体

- `.env` の `ACP_HOST_ROOT` の下にターゲットと `sessions.root` を、`ACP_SECRETS_DIR` の下にトークンや鍵のファイルを
  置く。コンテナにはホストと同じパスでマウントされる。
- ターゲットのイメージをビルドしておく（このリポジトリなら devcontainer をビルドする）。
- Claude Code の認証：`claude setup-token` を実行し、表示された OAuth トークンをファイルに保存して
  `claude.oauth_token_path` で指す（API キーなら `api_key_path`）。

### 2. エージェントごとの GitHub App（任意）

- エージェントごとに GitHub App を作り（Webhook はオフ）、ターゲットのリポジトリにインストールする。権限は
  ロールに合わせる：変更する developer は Contents と Pull requests を Read and write、読むだけの guide は
  Contents・Pull requests・Issues を Read-only。
- App ID、インストール ID（インストール設定画面の URL の末尾）、秘密鍵のファイルを `github` に書く。
- コミットする developer は、`git` に App の Bot ユーザーを書く。メールアドレスの番号は
  `gh api '/users/<App のスラッグ>[bot]' --jq .id` で調べる。

### 3. エージェントごとの Slack アプリ（任意）

- [deploy/slack-app-manifest.yaml](deploy/slack-app-manifest.yaml)（各項目の意味）からアプリを作り、名前を
  エージェントに合わせる。
- Basic Information で `connections:write` の App-Level Token を作り、ファイルに保存して `app_token_path` で指す。
- Bot トークン：
  - ローテーションあり（マニフェストの既定。Bot トークンが 12 時間で失効し、control-plane が更新する）：
    Basic Information の Client ID と Client Secret（ファイル）を `slack.rotation` に書き、下の
    「Slack のトークンを取り直す」で最初のトークンを取る。
  - ローテーションなし（マニフェストで `token_rotation_enabled: false`）：ワークスペースにインストールして得た
    Bot User OAuth Token を `bot_token_path` に置き、`slack.rotation` は書かない。
  - ローテーションなしで使っていたアプリを後から切り替えるときは、それまでの Bot トークンを `bot_token_path` に
    置いておけば初回に交換される。
- 依頼を許可する人の Slack のメンバー ID を `allowed_users` に書き、Bot を使うチャンネルに招待する。
- Bot トークンはエージェントにも渡り、エージェントは Bot が招待されたチャンネルを読み書きできる。

### 4. 起動

```bash
docker compose up -d --build
docker compose logs control-plane   # 起動できなかった Bot は "slack bot stopped" として理由が出る
```

### Slack のトークンを取り直す

ローテーションの状態（`acp-state` ボリューム）を失ったとき、更新できなくなったとき、最初のトークンがないときは、
OAuth の認可からトークンを取る。手順は次のコマンドが表示する。取ったあとは control-plane を再起動する。

```bash
docker compose run --rm control-plane slack-auth --config /etc/acp/acp.yaml --agent <名前>
```

## プラグイン

エージェントの共通の能力は `platform/` のプラグインにある。構成と、対話で使うときの準備は
[docs/platform/README.md](docs/platform/README.md) を参照。

## クライアントをビルドする

```bash
make build                                       # bin/acp（devcontainer 用）
GOOS=<ホストの OS> GOARCH=<ホストの CPU> make build   # ホスト用
```
