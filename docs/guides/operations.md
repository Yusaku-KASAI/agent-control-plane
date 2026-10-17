# 運用の手順

control-plane のサーバーを手元で準備して動かす手順。

## サーバーを準備して起動する

設定は [acp.example.yaml](../../acp.example.yaml)（各項目の意味）と [.env.example](../../.env.example) をもとに、`acp.yaml` と
`.env` を作る。どちらも gitignore の対象。以下は値の用意の仕方。

### 1. 全体

- ターゲットとその隣のマーケットプレイス（platform は `agent-platform`）を同じ親ディレクトリに並べ、その親ディレクトリを
  `.env` の `ACP_HOST_ROOT` の下に置く（並べ方は[ホストでの配置](../../template/docs/reference/contract.md#host "在りか")と
  [ホストの準備](../../template/docs/guides/devcontainer.md#host "在りか")）。platform はこのリポジトリの `platform/` にあるので、
  親ディレクトリでシンボリックリンクを張って隣に置く。

  ```bash
  cd <親ディレクトリ> && ln -s agent-control-plane/platform agent-platform
  ```

- `sessions.root` も `ACP_HOST_ROOT` の下に、トークンや鍵のファイルは `ACP_SECRETS_DIR` の下に置く（置ける場所は [.env.example](../../.env.example "在りか")）。
- ターゲットのイメージをビルドしておく（このリポジトリなら devcontainer をビルドする）。
- Claude Code の認証：`claude setup-token` を実行し、表示された OAuth トークンをファイルに保存して
  `claude.oauth_token_path` で指す（API キーなら `api_key_path`）。

### 2. エージェントごとの GitHub App（任意）

- エージェントごとに GitHub App を作り（Webhook はオフ）、ターゲットのリポジトリにインストールする。ロールごとに
  要る権限は [acp.example.yaml](../../acp.example.yaml "在りか") の `github` にある。
- App ID、インストール ID（インストール設定画面の URL の末尾）、秘密鍵のファイルを `github` に書く。
- コミットする developer は、`git` に App の Bot ユーザーを書く。メールアドレスの番号は
  `gh api '/users/<App のスラッグ>[bot]' --jq .id` で調べる。

### 3. エージェントごとの Slack アプリ（任意）

- [deploy/slack-app-manifest.yaml](../../deploy/slack-app-manifest.yaml)（各項目の意味）からアプリを作り、名前を
  エージェントに合わせる。
- Basic Information で `connections:write` の App-Level Token を作り、ファイルに保存して `app_token_path` で指す。
- Bot トークン：
  - ローテーションあり（マニフェストの既定。control-plane が更新する仕組みは[認証情報](../architecture/app/credentials.md "在りか")）：
    Basic Information の Client ID と Client Secret（ファイル）を `slack.rotation` に書き、下の
    「Slack のトークンを取り直す」で最初のトークンを取る。
  - ローテーションなし（マニフェストで `token_rotation_enabled: false`）：ワークスペースにインストールして得た
    Bot User OAuth Token を `bot_token_path` に置き、`slack.rotation` は書かない。
  - ローテーションなしで使っていたアプリを後から切り替えるときは、それまでの Bot トークンを `bot_token_path` に
    置いておけば初回に交換される。
- 依頼を許可する人の Slack のメンバー ID を `allowed_users` に書き、Bot を使うチャンネルに招待する。招待した
  チャンネルはエージェントも読み書きできる（[認証情報](../architecture/app/credentials.md "在りか")）。

### 4. 起動

`compose.yaml` は外部のネットワーク `agent-control-plane-net` を使い、自分では作らない。このリポジトリの devcontainer
も同じネットワークに入る（[コンテナの構成](../architecture/app/containers.md "在りか")）。control-plane か、このリポジトリの devcontainer を
初めて起動する前に、次で作る。

```bash
docker network inspect agent-control-plane-net >/dev/null 2>&1 || docker network create agent-control-plane-net
```

```bash
docker compose up -d --build
docker compose logs control-plane   # 起動できなかった Bot は "slack bot stopped" として理由が出る
```

## Slack のトークンを取り直す

ローテーションの状態（`acp-state` ボリューム）を失ったとき、更新できなくなったとき、最初のトークンがないときは、
OAuth の認可からトークンを取る。手順は次のコマンドが表示する。取ったあとは control-plane を再起動する。

```bash
docker compose run --rm control-plane slack-auth --config /etc/acp/acp.yaml --agent <名前>
```

## クライアントをビルドする

```bash
make build                                       # bin/acp（devcontainer 用）
GOOS=<ホストの OS> GOARCH=<ホストの CPU> make build   # ホスト用
```
