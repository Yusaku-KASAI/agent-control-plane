# agent-control-plane

Claude Code エージェントをコンテナで隔離して実行する制御基盤。

## 使い方

現在は CLI からエージェントに依頼できる。

```bash
acp run --agent <name> [--session <id>] [--config acp.yaml] "<指示>"
```

- `--session` を省くと新しいセッションを始める。ターゲットのリポジトリを
  `<sessions.root>/<セッションID>/workspace/<リポジトリ名>` に `git clone` する（コミット済みの内容だけが入る）。
- `--session <id>` を付けると、そのセッションの作業領域と会話を引き継いで続きを依頼する。
- ターンごとにターゲットのイメージからコンテナを作り、作業領域をマウントして、エージェントのロールで
  `claude -p` を実行する。コンテナは終了後に削除する。
- 結果を stdout に、セッション ID と作業領域のパスを stderr に表示する。

エージェントの変更を取り込むときは、作業領域で確認し、元のリポジトリから `git fetch <パス>` などで取得する。

### 準備

- Docker と git が使えるホスト上で実行する。
- ターゲットのイメージをビルドしておく（このリポジトリなら devcontainer をビルドする）。
- `claude.config_volume` に指定する Docker ボリュームに、Claude Code のログイン済み設定があること
  （devcontainer の `agent-control-plane_claude-code-config` を使える）。
- 設定ファイルは [acp.example.yaml](acp.example.yaml) をコピーして `acp.yaml` を作る。
  `sessions.root` はターゲットのリポジトリの外に置く（中に置くと設定エラーになる）。

### ビルド

```bash
GOOS=<ホストの OS> GOARCH=<ホストの CPU> make build   # bin/acp
```
