# CI

PR と main への push ごとに、devcontainer のイメージの中で `make check` と同じ検査を行う
（[ADR 0005](../adr/0005-ci-runs-in-devcontainer-image.md)）。あわせて control-plane のイメージがビルドできることを
確かめる（`check-image`）。ターゲットが守る検査の決まりは[契約](../reference/contract.md#checks)にある。

## 構成

```
Makefile                .devcontainer/Dockerfile   .github/workflows/ci.yml
check ─┬─ check-go       ◀─ check-go         ◀─ check-go       ジョブ ─┐
       ├─ check-platform ◀─ check-platform   ◀─ check-platform ジョブ ─┤
       ├─ check-target   ◀─ check-target     ◀─ check-target   ジョブ ─┼─▶ check ジョブ（必須）
       └─ check-docs     ◀─ check-docs       ◀─ check-docs     ジョブ ─┤
                            ルートの Dockerfile ◀─ check-image    ジョブ ─┘
```

◀─ は実行する向き（ジョブがステージをビルドし、ステージが `make` を実行する）、─▶ は結果をまとめる向き。

- 検査の中身は Makefile の `check-<関心>` にだけある。`make check` はそれらをまとめたもの
  （[ADR 0011](../adr/0011-ci-checks-as-cached-build-stages.md)）。
- Dockerfile の `check-<関心>` のステージは、その検査が読むファイルだけをコピーして `make check-<関心>` を実行する。
  それらが変わらなければ層のキャッシュが当たり、検査は実行されない。
- CI は関心ごとのジョブで同じ名前のステージを、イメージを書き出さずにビルドする（`check-image` も書き出さない）。
  層は GitHub Actions のキャッシュにジョブごとに残す。
- 結果は `check` ジョブにまとめる。必須のチェックは `check` だけ（[契約](../reference/contract.md#checks)）なので、
  ジョブを足しても ruleset は変えない。
- キャッシュが当たると新しく公開された脆弱性を拾えないので、週に 1 回、`check-go` だけキャッシュを使わずに実行する。

## 構成要素

### `Makefile`

- **責務**：検査の中身を関心ごとの `check-<関心>` として定義し、`check` で束ねる。
- **置くもの / 置かないもの**：`check-go`（lint、`go mod tidy` の差分、govulncheck、テスト）、`check-platform`
  （プラグインの検証と shellcheck）、`check-target`（devcontainer のスクリプトの shellcheck と VS Code の設定の比較）、
  `check-docs`。ほかに `fmt`、`test`、`build` のような手元の作業用のターゲットも置くが、`check` には含めない。
- **関係**：人も CI も同じターゲットを実行する。`check-target` と `check-docs` はどのターゲットでも共通
  （[契約](../reference/contract.md)）。
- **当てはめた原則**：[単一の情報源](../../../platform/docs/reference/principles.md#ssot)：検査のコマンドはここにだけ書き、
  Dockerfile と CI は `make` で呼ぶ。
  [関心の分離](../../../platform/docs/reference/principles.md#soc)：関心ごとに分け、読むファイルの範囲を区切る。

### `check-*` のステージ（`.devcontainer/Dockerfile`）

- **責務**：関心ごとに、その検査が読むファイルだけをコピーして `make check-<関心>` を実行する。
- **置くもの / 置かないもの**：
  - `check-base` は devcontainer のイメージ（ツールチェーンのステージ）を元に、Makefile だけをコピーする。各
    `check-<関心>` はこれを元にする。
  - `check-go` は依存の取得（`go mod download`）を先に一つの層にし、そのあと lint の設定、例の設定、プラグインの
    宣言（`.claude`）、`platform`、Go のコードをコピーする。テストがそれらも読むため。
  - `check-docs` は、リンクがどのファイルを指してもよいので、文脈をすべてコピーする。
  - ステージに見えるのはコピーしたファイルだけなので、コピーにないファイルは検査から読めず、変えてもキャッシュを
    外さない。
- **関係**：ビルドの文脈は `Dockerfile.dockerignore` で決まる（[devcontainer](devcontainer.md)）。
- **当てはめた原則**：[高凝集・疎結合](../../../platform/docs/reference/principles.md#cohesion)：検査と、それが読むファイルを
  一つのステージにまとめ、ほかのファイルの変更がキャッシュを外さない。
  [仕組みで守る](../../../platform/docs/reference/principles.md#mechanisms)：devcontainer と同じイメージで検査するので、
  手元と CI で道具がずれない。

### `.github/workflows/ci.yml`

- **責務**：`check-<関心>` ごとのジョブでステージをビルドし、結果を `check` ジョブにまとめる。
- **置くもの / 置かないもの**：
  - 行列のジョブ（`check-go`、`check-platform`、`check-target`、`check-docs`、`check-image`）。一つが失敗してもほかは
    最後まで動かす。`check-image` はルートの Dockerfile（control-plane のイメージ）をビルドする。
  - ビルドは `outputs: type=cacheonly` でイメージを書き出さない。キャッシュは GitHub Actions のキャッシュに、ジョブごとの
    scope で残す（同じ scope に書くジョブどうしが互いのキャッシュを上書きするため）。
  - 毎週月曜 0 時（UTC）の定期実行では、`check-go` のステージだけキャッシュを使わない。
  - `check` ジョブは前のジョブの結果にかかわらず必ず動き、すべて成功していなければ失敗する。前のジョブの失敗で
    飛ばされたジョブは成功と数えられてしまうため。
  - 検査のコマンドは置かない。
- **関係**：ターゲットごとに埋めるのは、行列の `check-<関心>` とアプリのイメージの検査（[契約](../reference/contract.md)）。
- **当てはめた原則**：[仕組みで守る](../../../platform/docs/reference/principles.md#mechanisms)：必須のチェックを `check`
  一つにし、ジョブの増減で ruleset を変えずに済ませる。
  [Docs as Code](../../../platform/docs/reference/principles.md#docs-as-code)：文書もコードと同じ PR の検査を通す。

### `tools/check-docs.py`

- **責務**：Markdown の相対リンクと ADR の形を確かめる。
- **置くもの / 置かないもの**：確かめる中身は platform の[文書の形式](../../../platform/docs/reference/formats.md)の各節に
  ある。対象は git が追う Markdown で、CI のビルドの文脈のように `.git` がないときは、文脈にあるすべての Markdown を
  対象にする。
- **関係**：`make check-docs` が実行する。どのターゲットでも共通（[契約](../reference/contract.md)）。
- **当てはめた原則**：[Docs as Code](../../../platform/docs/reference/principles.md#docs-as-code)：リンク切れと ADR の
  食い違いを機械で見つける。
  [メタデータ](../../../platform/docs/reference/principles.md#metadata)：ADR の状態と参照を frontmatter から読んで確かめる。
