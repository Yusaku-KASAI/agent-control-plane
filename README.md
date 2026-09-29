# agent-control-plane

Claude Code エージェントをコンテナで隔離して実行する制御基盤。

## 使い方

現在は CLI から 1 回だけエージェントを実行できる。

```bash
acp run --agent <name> [--config acp.yaml] "<指示>"
```

指定したエージェントのターゲットのイメージからコンテナを作り、ターゲットのリポジトリをマウントして
`claude -p` を実行し、結果を表示する。コンテナは終了後に削除する。

### 準備

- Docker が動くホスト上で実行する。
- ターゲットのイメージをビルドしておく（このリポジトリなら devcontainer をビルドする）。
- `claude.config_volume` に指定する Docker ボリュームに、Claude Code のログイン済み設定があること
  （devcontainer の `agent-control-plane_claude-code-config` を使える）。
- 設定ファイルは [acp.example.yaml](acp.example.yaml) をコピーして `acp.yaml` を作る。

### ビルド

```bash
GOOS=<ホストの OS> GOARCH=<ホストの CPU> make build   # bin/acp
```
