# 運用の手順

control-plane のサーバーを手元で準備して動かす手順。

## サーバーを準備して起動する

設定は [acp.example.yaml](../../acp.example.yaml)（各項目の意味）と [.env.example](../../.env.example) をもとに、`acp.yaml` と
`.env` を作る。どちらも gitignore の対象。以下は値の用意の仕方。

### 1. 全体

- ターゲットとその隣のマーケットプレイス（platform は `agent-platform`）を同じ親ディレクトリに並べ、その親ディレクトリを
  `.env` の `ACP_HOST_ROOT` の下に置く（並べ方は[ホストでの配置](../../platform/docs/reference/target.md#host "在りか")と
  [ホストの準備](../../template/docs/guides/devcontainer.md#host "在りか")）。platform はこのリポジトリの `platform/` にあるので、
  親ディレクトリでシンボリックリンクを張って隣に置く。

  ```bash
  cd <親ディレクトリ> && ln -s agent-control-plane/platform agent-platform
  ```

- `sessions.root` と `usage_log` も `ACP_HOST_ROOT` の下に、トークンや鍵のファイルは `ACP_SECRETS_DIR` の下に置く（置ける場所は [.env.example](../../.env.example "在りか")）。
- ターゲットのイメージをビルドしておく（このリポジトリなら devcontainer をビルドする）。
- ターゲットのリポジトリの既定のブランチを、ルールセットで守っておく（[既定のブランチの守りの用意](../../template/docs/guides/protection.md "在りか")）。
- Claude Code の認証：`claude setup-token` を実行し、表示された OAuth トークンをファイルに保存して
  `claude.oauth_token_path` で指す（API キーなら `api_key_path`）。

### 2. エージェントごとの GitHub App（任意）

- エージェントごとに GitHub App を作り（Webhook はオフ）、そのエージェントのターゲットのリポジトリだけにインストールする
  （Repository access で「Only select repositories」を選ぶ）。トークンはインストールの範囲のすべてに届くので、
  「All repositories」で入れると、守りのないほかのリポジトリにも書き込める
  （[ADR 0053](../adr/app/0053-limit-github-app-reach-by-installation.md)）。ロールごとに
  要る権限は[ハーネスの契約の認証情報](../../platform/docs/reference/harness.md#credentials "在りか")にある。
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
    「Slack のトークンを取り直す」で最初のトークンを取る。ローテーションありでも `bot_token_path` は
    書く（何を置くかは [acp.example.yaml](../../acp.example.yaml "在りか")）。
  - ローテーションなし（マニフェストで `token_rotation_enabled: false`）：ワークスペースにインストールして得た
    Bot User OAuth Token を `bot_token_path` に置き、`slack.rotation` は書かない。
- 依頼を許可する人の Slack のメンバー ID を `allowed_users` に書き、Bot を使うチャンネルに招待する。招待した
  チャンネルはエージェントも読み書きできる（[認証情報](../architecture/app/credentials.md "在りか")）。

### 4. 起動

```bash
docker compose up -d --build
docker compose logs control-plane   # 起動できなかった Bot は "slack bot stopped" として理由が出る
```

API はホストの `127.0.0.1:8787` で届く。このリポジトリの devcontainer から試すときだけ、ホストのリポジトリの
ディレクトリでその場で control-plane のネットワークにつなぎ、試し終えたら外す。

```bash
dev=$(docker compose -f .devcontainer/compose.yaml ps -q dev-container)
docker network connect agent-control-plane_default "$dev"
```

つないでいる間は、devcontainer の中でトークンを渡して依頼する（[クライアントをビルドする](#build-client)）。トークンは
シェルの履歴に残らないよう、コマンド行に書かずに読み込み、試し終えたら消す。

```bash
read -rs ACP_API_TOKEN && export ACP_API_TOKEN
bin/acp run --server http://control-plane:8787 --agent <name> "<指示>"
unset ACP_API_TOKEN
```

試し終えたら、ホストで外す。

```bash
docker network disconnect agent-control-plane_default "$dev"
```

## Slack のトークンを取り直す

ローテーションの状態（`acp-state` ボリューム）を失ったとき、更新できなくなったとき、最初のトークンがないときは、
OAuth の認可からトークンを取る。手順は次のコマンドが表示する。取ったあとは control-plane を再起動する。

```bash
docker compose run --rm control-plane slack-auth --config /etc/acp/acp.yaml --agent <名前>
```

<a id="usage"></a>

## 使用量を見る

ターンごとの使用量は `usage_log` のファイルに 1 行ずつ残る（項目は[使用量の記録](../architecture/app/sessions.md#usage "在りか")）。
たとえば、エージェントと依頼者ごとの見積もりの費用は次で足し合わせる。使用量の分からないターン（`usage` が `null`）は
入らない。

```bash
jq -s 'map(select(.usage != null)) | group_by([.agent, (.requesters | map(.id) | join(","))])
  | map({agent: .[0].agent, requesters: (.[0].requesters | map(.id)), turns: length, cost_usd: (map(.usage.cost_usd) | add)})' <usage_log のパス>
```

<a id="build-client"></a>

## クライアントをビルドする

```bash
make build                                       # bin/acp（devcontainer 用）
GOOS=<ホストの OS> GOARCH=<ホストの CPU> make build   # ホスト用
```
