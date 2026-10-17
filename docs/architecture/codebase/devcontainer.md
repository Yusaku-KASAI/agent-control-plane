# このリポジトリの devcontainer と検査

target-template の環境のファイルのうち、このリポジトリが埋める部分（[ファイルの分類](../../../template/docs/overview.md#files)）。
Dockerfile の Go のツールチェーンと検査のステージ、compose.yaml の control-plane のネットワーク、Makefile の検査の
ルール、CI のジョブ、devcontainer.json と `.vscode/settings.json` の Go の部分を扱う。共通の部分の構成は target-template の
[devcontainer](../../../template/docs/architecture/devcontainer.md "在りか") と [CI](../../../template/docs/architecture/ci.md) にあり、
ルール（make のルール）という言葉も CI の文書のとおりに使う。

## 構成

```
Dockerfile のステージ（矢印は元にするか、コピーする向き）
  base ─▶ go ─▶ check-base ─▶ check-go、check-platform（このリポジトリの検査）
  go-source ─▶ go-tools ─▶ go
  go-source ─▶ go

Makefile                .devcontainer/Dockerfile   .github/workflows/ci.yml
check ─┬─ check-go       ◀─ check-go         ◀─ check-go       ジョブ ─┐
       ├─ check-platform ◀─ check-platform   ◀─ check-platform ジョブ ─┤
       ├─ check-target   ◀─ check-target     ◀─ check-target   ジョブ ─┼─▶ check ジョブ
       └─ check-docs     ◀─ check-docs       ◀─ check-docs     ジョブ ─┤
                            ルートの Dockerfile ◀─ check-image    ジョブ ─┘
```

- ツールチェーンのステージは `go`。devcontainer のイメージ（`agent-control-plane-devcontainer`）になり、
  control-plane はこのリポジトリのエージェントも同じイメージで動かす（設定の `agents[].target.image`。例は [acp.example.yaml](../../../acp.example.yaml "在りか")）。
- 検査の関心は、共通の `check-target` と `check-docs` に、`check-go` と `check-platform` を足したもの。CI には、
  control-plane のイメージがビルドできることを確かめる `check-image` を足す。

## 構成要素

### Dockerfile の `go-source` ステージ

- **責務**：版を固定した Go のイメージに名前を付け、`go-tools` と `go` が `COPY --from` で使えるようにする。

### Dockerfile の `go-tools` ステージ

- **責務**：Go のツール（gopls、delve、govulncheck、golangci-lint）を、base と切り離してビルドする。
- **関係**：バイナリだけを `go` の `/usr/local/bin` にコピーする。GOPATH の下は一部がボリュームなので置かない
  （版がここで固定したものからずれる）。
- **当てはめた原則・パターン**：[高凝集・疎結合](../../../platform/plugins/principles/docs/reference/principles.md#cohesion)：base を変えても
  （Claude Code を上げても）ツールを作り直さない。

### Dockerfile の `go` ステージ

- **責務**：ツールチェーンのステージとして、Go とそのツールを base に足す。
- **関係**：compose.yaml がこのステージをビルドする。Go のモジュールとビルドのキャッシュは、compose.yaml の名前付き
  ボリューム（`go-mod`、`go-build`）に置く。
- **当てはめた原則・パターン**：[関心の分離](../../../platform/plugins/principles/docs/reference/principles.md#soc)：このリポジトリのツールチェーンを、
  共有する base から分ける。

### compose.yaml の control-plane のネットワーク

- **責務**：devcontainer から control-plane を試せるよう、devcontainer を control-plane のネットワーク
  `agent-control-plane-net` に入れ、`ACP_SERVER=http://control-plane:8787` で API を指す。control-plane を開発する
  ターゲットだけが埋める部分（[ターゲットの契約](../../../template/docs/reference/contract.md#control-plane "在りか")）。
- **置くもの / 置かないもの**：API のトークン（`ACP_API_TOKEN`）は置かない。試すときに人が手で渡す。
- **関係**：ネットワークは外部のもので、compose は作らない。devcontainer を初めて起動する前に、
  [運用の手順](../../guides/operations.md#setup-overall "在りか")の 1 段で作る。control-plane の側は[コンテナの構成](../app/containers.md "在りか")にある。

### `check-go`（ルール、ステージ、CI のジョブ）

- **責務**：Go のコードの lint、`go mod tidy` の差分、govulncheck、テストを行う。中身は Makefile のルールにあり、
  同じ名前のステージと CI のジョブがそれを呼ぶ。
- **置くもの / 置かないもの**：ステージは依存の取得（`go mod download`）を先に一つの層にし、そのあと lint の設定、
  例の設定、プラグインの宣言（`.claude`）、`platform`、Go のコードをコピーする。テストがそれらも読むため。
- **関係**：キャッシュが当たると新しく公開された脆弱性を拾えないので、CI は週に 1 回、`check-go` のステージだけ
  キャッシュを使わずにビルドする。
- **当てはめた原則・パターン**：[高凝集・疎結合](../../../platform/plugins/principles/docs/reference/principles.md#cohesion)：変わりにくい依存の層を
  先に置き、コードを変えても依存の取得はやり直さない。

### `check-platform`（ルール、ステージ、CI のジョブ）

- **責務**：`platform/` のプラグインを検証し、そのスクリプトを shellcheck にかける。

### CI の `check-image`

- **責務**：ルートの `Dockerfile`（control-plane のイメージ）がビルドできることを確かめる。
- **関係**：ルートの `.dockerignore` を使う。`.devcontainer/Dockerfile` の文脈は `Dockerfile.dockerignore` で、
  互いに使わない。

### devcontainer.json と `.vscode/settings.json` の Go の部分

- **責務**：Go の拡張機能と、その設定を持つ。
- **置くもの / 置かないもの**：ツールはイメージで固定するので、拡張機能が自分でツールを入れたり更新したりしないようにする。
