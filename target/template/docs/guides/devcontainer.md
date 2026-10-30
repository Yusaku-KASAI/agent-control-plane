# devcontainer の使い方

devcontainer をホストに用意して、ビルド、起動、停止する手順。仕組み（イメージのステージ、compose、マウント）は
[architecture/devcontainer.md](../architecture/devcontainer.md "在りか") にある。

<a id="host"></a>

## ホストの準備

ターゲットと platform のマーケットプレイスを、[ターゲットの契約の配置](../../../../platform/docs/reference/target.md#host)のとおりに並べる。

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
