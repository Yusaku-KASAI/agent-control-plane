# agent-control-plane

Claude Code エージェントをコンテナで隔離して実行する制御基盤。

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

## サーバーを動かす

```bash
cp .env.example .env    # 値を埋める
docker compose up -d --build
```

- `acp.yaml` は [acp.example.yaml](acp.example.yaml) をもとに作り、`.env` の `ACP_CONFIG` で指す。
  `sessions.root` はターゲットのリポジトリの外に置く（中に置くと設定エラーになる）。
- `acp.yaml` に書くホストのパスは、ターゲットと `sessions.root` を `ACP_HOST_ROOT` の下に、秘密鍵を
  `ACP_SECRETS_DIR` の下に置く。コンテナには同じパスでマウントされる。
- ターゲットのイメージをビルドしておく（このリポジトリなら devcontainer をビルドする）。
- `claude.config_volume` の Docker ボリュームに、Claude Code のログイン済み設定があること
  （devcontainer の `agent-control-plane_claude-code-config` を使える）。
- エージェントに push と PR の作成をさせる場合は、エージェントごとに GitHub App を作る。
  - 権限は Contents と Pull requests を Read and write にし、ターゲットのリポジトリにインストールする。
  - App ID、インストール ID（インストール設定画面の URL の末尾）、秘密鍵のファイルを `github` に設定する。
  - コミットの作者 `git` には App の Bot ユーザーを設定する。メールアドレスの ID は App ID ではなく Bot ユーザーの
    アカウント ID で、`gh api '/users/<App のスラッグ>[bot]' --jq .id` で調べられる。
  - `github` を省くとトークンは渡さず、エージェントはコミットまでにとどめる。
- Slack から依頼する場合は、エージェントごとに Slack アプリを作る。
  - [deploy/slack-app-manifest.yaml](deploy/slack-app-manifest.yaml) から作り、名前をエージェントに合わせる。
  - Basic Information で `connections:write` の App-Level Token を作り、ワークスペースにインストールして
    Bot User OAuth Token を得る。それぞれをファイルに保存し、`slack` の `app_token_path` と `bot_token_path` に設定する。
  - 依頼を許可する人の Slack のメンバー ID を `allowed_users` に書く。
  - `people` に Slack のメンバー ID と git の名前・メールアドレスを書いた人は、その人の依頼で作られたコミットに
    `Co-authored-by` として記録される。
  - Bot トークンはエージェントにも渡り、エージェントは Slack のメッセージを読み書きできる。Bot が読めるのは
    招待されたチャンネルだけ。

## プラグイン

エージェントの共通の能力は `platform/` のプラグインにある。構成と、対話で使うときの準備は
[docs/platform/README.md](docs/platform/README.md) を参照。

## クライアントをビルドする

```bash
make build                                       # bin/acp（devcontainer 用）
GOOS=<ホストの OS> GOARCH=<ホストの CPU> make build   # ホスト用
```
