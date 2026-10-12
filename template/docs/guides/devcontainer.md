# devcontainer の使い方

<a id="host"></a>

## ホストの準備

ターゲットと platform のマーケットプレイスを、[ターゲットの契約の配置](../reference/contract.md#host)のとおりに並べる。
切り出すまでは、ターゲットを並べたディレクトリで次を実行して、agent-control-plane の `platform/` を指すリンクを置く。

```bash
ln -s agent-control-plane/platform agent-platform
```

## ビルド、起動、停止

devcontainer のビルド、起動、停止はコマンドで行う（VS Code の「Rebuild Container」は使わない）。

```bash
.devcontainer/init-host.sh                                   # ホストの準備（最初に一度。何度実行してもよい）
docker compose -f .devcontainer/compose.yaml up -d --build   # ビルドして起動する
docker compose -f .devcontainer/compose.yaml down            # 止める
```

起動したあと、VS Code の「Reopen in Container」で入る。

`init-host.sh` が書いた値を変えるときは、`.devcontainer/.env` を直してからビルドし直す。項目は
`.devcontainer/.env.example` にある。

<a id="root"></a>

## root で作業する

コンテナは root を使わないが、人が一時的に root で作業したいとき（apt でのインストールなど）は
`docker exec -u root -it <コンテナ> bash` で入る。

<a id="permissions"></a>

## Claude Code の権限の確認

対話の既定のモードと、確認せずに使えるツールの一覧は `.claude/settings.json` の `permissions` にある。コンテナで
隔離しているので、確認を省いて使ってもよい。

```bash
claude --dangerously-skip-permissions
```

<a id="copy"></a>

## Claude Code の画面のコピー

Claude Code のフルスクリーン表示は、マウスで選択して離すと自動でコピーする。止めるには `/config` の Copy on select を
オフにする（設定は Claude Code の設定ディレクトリに残る）。そのあとは選択中に `Ctrl+C` か `Ctrl+Shift+C` でコピーする。

仕組みは [architecture/devcontainer.md](../architecture/devcontainer.md) にある。
