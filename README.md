# agent-control-plane

Claude Code エージェントをコンテナで隔離して実行する制御基盤。

## 構成

- `acp serve`：依頼を HTTP API で受け、エージェントのコンテナを動かすサーバー。`compose.yaml` でコンテナとして
  動かす。Docker へは、使う API だけを通すプロキシ経由でアクセスする。
- `acp run`：サーバーに依頼して結果を表示するクライアント。

## 依頼する

```bash
acp run --agent <name> [--session <id>] [--server <url>] "<指示>"
```

- サーバーの URL は `--server`、`ACP_SERVER`、`http://127.0.0.1:8787` の順で決まる。API トークンは
  `ACP_API_TOKEN` から読む。
- `--session` を省くと新しいセッションを始める。ターゲットのリポジトリを
  `<sessions.root>/<セッションID>/workspace/<リポジトリ名>` に `git clone` する。
- `--session <id>` を付けると、そのセッションの作業領域と会話を引き継いで続きを依頼する。
- 作業領域では `acp/<セッションIDの先頭8文字>` ブランチで作業し、`origin` は元のリポジトリと同じリモートを指す。
  GitHub App があればリモートのデフォルトブランチから、なければ元のリポジトリの HEAD からブランチを切る。
- ターンごとに GitHub App のトークンを発行し、ターゲットのイメージからコンテナを作り、作業領域をマウントして、
  エージェントのロールで `claude -p` を実行する。コンテナは終了後に削除する。
- 結果を stdout に、セッション ID と作業領域のパスを stderr に表示する。

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
  - `github` を省くとトークンは渡さず、エージェントはコミットまでにとどめる。

## クライアントをビルドする

```bash
make build                                       # bin/acp（devcontainer 用）
GOOS=<ホストの OS> GOARCH=<ホストの CPU> make build   # ホスト用
```
