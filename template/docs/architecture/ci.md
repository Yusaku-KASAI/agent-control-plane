# CI

PR と main への push ごとに、devcontainer のイメージの中で `make check` と同じ検査を行う
（[ADR 0005](../adr/0005-ci-runs-in-devcontainer-image.md)）。ターゲットが守る検査の決まりは[契約](../reference/contract.md#checks)にある。

## 構成

```
Makefile                .devcontainer/Dockerfile   .github/workflows/ci.yml
check ─┬─ check-<関心>   ◀─ check-<関心>     ◀─ check-<関心>   ジョブ ─┐
       ├─ check-target   ◀─ check-target     ◀─ check-target   ジョブ ─┼─▶ check ジョブ（必須）
       └─ check-docs     ◀─ check-docs       ◀─ check-docs     ジョブ ─┤
                            ほかの Dockerfile  ◀─ イメージのビルドのジョブ ─┘
```

◀─ は実行する向き（ジョブがステージをビルドし、ステージが `make` を実行する）、─▶ は結果をまとめる向き。

- 検査の中身は Makefile の `check-<関心>` にだけあり、Dockerfile のステージと CI のジョブはそれを呼ぶ
  （[ADR 0011](../adr/0011-ci-checks-as-cached-build-stages.md)）。
- 関心ごとに、Makefile のターゲット、Dockerfile のステージ、CI のジョブが同じ名前で一つずつ並ぶ。
- ターゲットにほかのイメージ（アプリをデプロイするためのイメージなど）があれば、そのビルドも CI のジョブにする。
- CI のジョブの結果は `check` ジョブにまとまる。

## 構成要素

### `Makefile`

- **責務**：検査の中身を関心ごとの `check-<関心>` として定義し、`check` で束ねる。
- **置くもの / 置かないもの**：`check-target`（devcontainer のスクリプトの shellcheck と VS Code の設定の比較）、
  `check-docs`、ターゲットの関心ごとの `check-<関心>`。手元の作業用のターゲットも置いてよいが、`check` には含めない。
- **関係**：人も CI も同じターゲットを実行する。`check-target` と `check-docs` はどのターゲットでも共通
  （[契約](../reference/contract.md#files)）。
- **当てはめた原則・パターン**：[単一の情報源](../../../platform/plugins/principles/docs/reference/principles.md#ssot)：検査のコマンドはここにだけ書き、
  Dockerfile と CI は `make` で呼ぶ。
  [関心の分離](../../../platform/plugins/principles/docs/reference/principles.md#soc)：関心ごとに分け、読むファイルの範囲を区切る。

### `check-*` のステージ（`.devcontainer/Dockerfile`）

- **責務**：関心ごとに、その検査が読むファイルだけをコピーして `make check-<関心>` を実行する。
- **置くもの / 置かないもの**：
  - `check-base` は devcontainer のイメージ（ツールチェーンのステージ）を元に、Makefile だけをコピーする。各
    `check-<関心>` はこれを元にする。
  - `check-docs` は、リンクがどのファイルを指してもよいので、文脈をすべてコピーする。
  - ステージに見えるのはコピーしたファイルだけなので、コピーにないファイルは検査から読めず、変えてもキャッシュを
    外さない。読むファイルが変わらなければ層のキャッシュが当たり、検査は実行されない。
- **関係**：ビルドの文脈は `Dockerfile.dockerignore` で決まる（[devcontainer](devcontainer.md)）。
- **当てはめた原則・パターン**：[高凝集・疎結合](../../../platform/plugins/principles/docs/reference/principles.md#cohesion)：検査と、それが読むファイルを
  一つのステージにまとめ、ほかのファイルの変更がキャッシュを外さない。
  [単一の情報源](../../../platform/plugins/principles/docs/reference/principles.md#ssot)：devcontainer と同じイメージを元にするので、
  手元と CI で道具がずれない。

### `.github/workflows/ci.yml`

- **責務**：`check-<関心>` ごとのジョブでステージをビルドし、結果を `check` ジョブにまとめる。
- **置くもの / 置かないもの**：
  - 行列のジョブ（`check-<関心>` と、ほかのイメージのビルド）。一つが失敗してもほかは最後まで動かす。
  - ビルドは `outputs: type=cacheonly` でイメージを書き出さない。キャッシュは GitHub Actions のキャッシュに、ジョブごとの
    scope で残す（同じ scope に書くジョブどうしが互いのキャッシュを上書きするため）。
  - 毎週の定期実行では、選んだ検査のステージだけキャッシュを使わない。キャッシュが当たると、新しく公開された
    脆弱性を拾えないため。
  - `check` ジョブは前のジョブの結果にかかわらず必ず動き、すべて成功していなければ失敗する。前のジョブの失敗で
    飛ばされたジョブは成功と数えられてしまうため。必須のチェックはこれだけ（[契約](../reference/contract.md#checks)）。
  - 検査のコマンドは置かない。
- **関係**：ターゲットごとに埋めるのは、行列のジョブと、キャッシュを使わない検査（[契約](../reference/contract.md#files)）。
- **当てはめた原則・パターン**：[高凝集・疎結合](../../../platform/plugins/principles/docs/reference/principles.md#cohesion)：ruleset が知るのは
  `check` だけなので、ジョブを足しても ruleset は変えない。
  [Docs as Code](../../../platform/plugins/principles/docs/reference/patterns.md#docs-as-code)：文書もコードと同じ PR の検査を通す。

### `tools/check-docs.py`

- **責務**：文書の形式のうち機械で確かめられるものを確かめる。中身は conventions の[文書の体系](../../../platform/plugins/conventions/docs/architecture/documentation.md)の構成要素の説明にある。
- **関係**：`make check-docs` が実行する。どのターゲットでも共通（[契約](../reference/contract.md#files)）。
