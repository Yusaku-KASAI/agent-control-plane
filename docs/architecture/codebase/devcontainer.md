# このリポジトリの devcontainer と検査

target-template の環境のファイルのうち、このリポジトリが埋める部分（[ファイルの分類](../../../template/docs/overview.md#files)）。
Dockerfile の Go のツールチェーンと検査のステージ、Makefile の検査のルール、CI のジョブ、devcontainer.json と
`.vscode/settings.json` の Go の部分を扱う。共通の部分の構成は target-template の
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
       └─ check-docs（手元）                                           │
                            ルートの Dockerfile ◀─ check-image ジョブ ─┤
                            check-docs アクション ◀─ check-docs ジョブ ─┤
                      check-protection アクション ◀─ check-protection ジョブ ─┘
```

- ツールチェーンのステージは `go`。devcontainer のイメージ（`agent-control-plane-devcontainer`）になり、
  control-plane はこのリポジトリのエージェントも同じイメージで動かす（設定の `agents[].target.image`。例は [acp.example.yaml](../../../acp.example.yaml "在りか")）。
- 検査の関心は、共通の `check-target` に、`check-go`、`check-platform`、`check-docs` を足したもの。`check-docs` は、
  このリポジトリにある conventions の文書の検査（`platform/plugins/conventions/bin/check-docs`）を回す。手元では
  `make check-docs` で、CI ではターゲットと同じく conventions の複合アクションで回す。CI では `@main` ではなく
  リポジトリの中のパスで呼び、PR の版のアクションを確かめる（[template の ADR 0014](../../../template/docs/adr/0014-target-independent-checks-as-platform-actions.md)）。`check-protection`
  ジョブも同じく、リポジトリの中のパスで github の複合アクションを呼ぶ（中のパスで呼ぶため、チェックアウトしてから）。CI には、
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
