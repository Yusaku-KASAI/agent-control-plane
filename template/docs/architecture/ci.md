# CI

PR と main への push のたびと、毎週の定期実行で、devcontainer のイメージの中で `make check` と同じ検査を行う
（[ADR 0005](../adr/0005-ci-runs-in-devcontainer-image.md)）。あわせて、ターゲットに依存しない文書の検査を、platform の
複合アクションで devcontainer の外で行う（[ADR 0014](../adr/0014-target-independent-checks-as-platform-actions.md)）。既定のブランチの守りが外れていないことも、同じ形で確かめる
（[ADR 0016](../adr/0016-protection-checked-in-ci.md)）。ターゲットが用意する検査は[ターゲットの契約](../reference/contract.md#checks)にある。
Makefile の `check-<関心>` のように `make` で実行する単位を、make のルール（以下、ルール）と呼ぶ。

## 構成

```
Makefile                .devcontainer/Dockerfile   .github/workflows/ci.yml
check ─┬─ check-<関心>   ◀─ check-<関心>     ◀─ check-<関心>   ジョブ ─┐
       └─ check-target   ◀─ check-target     ◀─ check-target   ジョブ ─┼─▶ check ジョブ（必須）
                            ほかの Dockerfile  ◀─ イメージのビルドのジョブ ─┤
                            check-docs アクション ◀─ check-docs ジョブ ─┤
                      check-protection アクション ◀─ check-protection ジョブ ─┘
```

◀─ は実行する向き（ジョブがステージをビルドし、ステージが `make` を実行する。`check-docs` ジョブはアクションを呼ぶ）、
─▶ は結果をまとめる向き。

- ターゲットの検査の中身は Makefile の `check-<関心>` にだけあり、Dockerfile のステージと CI のジョブはそれを呼ぶ
  （[ADR 0011](../adr/0011-ci-checks-as-cached-build-stages.md)）。
- ターゲットの関心ごとに、Makefile のルール、Dockerfile のステージ、CI のジョブが同じ名前で一つずつ並ぶ。
- ターゲットにほかのイメージ（アプリをデプロイするためのイメージなど）があれば、そのビルドも CI のジョブにする。
- CI のジョブの結果は `check` ジョブにまとまる。
- 文書の形式の検査は conventions プラグインのコマンドで、`make check` には含めない。CI では `check-docs` ジョブが
  conventions の複合アクションを呼ぶ。エージェントが Markdown を変えたターンの終わりには、conventions の hook も回す
  （[conventions の全体像](../../../platform/plugins/conventions/docs/overview.md "在りか")）。

## 構成要素

### `Makefile`

- **責務**：検査の中身を関心ごとの `check-<関心>` として定義し、`check` で束ねる。
- **置くもの / 置かないもの**：`check-target`（devcontainer のスクリプトの shellcheck と VS Code の設定の比較）と、
  ターゲットの関心ごとの `check-<関心>`。手元の作業用のルールも置いてよいが、`check` には含めない。
- **関係**：人も CI も同じルールを実行する。`check-target` はどのターゲットでも共通
  （[ファイルの分類](../overview.md#files)）。
- **当てはめた原則・パターン**：[単一の情報源](../../../platform/plugins/principles/docs/reference/principles.md#ssot)：検査のコマンドはここにだけ書き、
  Dockerfile と CI は `make` で呼ぶ。
  [関心の分離](../../../platform/plugins/principles/docs/reference/principles.md#soc)：関心ごとに分け、読むファイルの範囲を区切る。

### `check-*` のステージ（`.devcontainer/Dockerfile`）

- **責務**：関心ごとに、その検査が読むファイルだけをコピーして `make check-<関心>` を実行する。
- **置くもの / 置かないもの**：
  - `check-base` は devcontainer のイメージ（ツールチェーンのステージ）を元に、Makefile だけをコピーする。各
    `check-<関心>` はこれを元にする。
  - ステージに見えるのはコピーしたファイルだけなので、コピーにないファイルは検査から読めず、変えてもキャッシュを
    外さない。読むファイルが変わらなければ層のキャッシュが当たり、検査は実行されない。
- **関係**：ビルドの文脈は `Dockerfile.dockerignore` で決まる（[devcontainer](devcontainer.md "在りか")）。
- **当てはめた原則・パターン**：[高凝集・疎結合](../../../platform/plugins/principles/docs/reference/principles.md#cohesion)：検査と、それが読むファイルを
  一つのステージにまとめ、ほかのファイルの変更がキャッシュを外さない。
  [単一の情報源](../../../platform/plugins/principles/docs/reference/principles.md#ssot)：devcontainer と同じイメージを元にするので、
  手元と CI で道具がずれない。

### `.github/workflows/ci.yml`

- **責務**：`check-<関心>` ごとのジョブでステージをビルドし、文書の検査のジョブとあわせて、結果を `check` ジョブにまとめる。
- **置くもの / 置かないもの**：
  - 行列のジョブ（`check-<関心>` と、ほかのイメージのビルド）。一つが失敗してもほかは最後まで動かす。
  - `check-docs` ジョブ。`uses: Yusaku-KASAI/agent-control-plane/platform/plugins/conventions/actions/check-docs@main`
    の一つのステップだけを持つ（チェックアウトもアクションが行う）。中身は platform に置き、ターゲットには書かない。
    ほかのリポジトリのコードを動かすので、ジョブの `permissions` は `contents: read` だけにする。
  - `check-protection` ジョブ。`uses: Yusaku-KASAI/agent-control-plane/platform/plugins/github/actions/check-protection@main`
    の一つのステップだけを持ち、`permissions` は `contents: read` だけにする。リポジトリのファイルではなく設定を読むので、
    設定が変われば PR の変更と関わりなく落ちる。何を確かめるかは[既定のブランチの守りの用意](../guides/github.md#verify "在りか")にある。
  - ビルドは `outputs: type=cacheonly` でイメージを書き出さない。キャッシュは GitHub Actions のキャッシュに、ジョブごとの
    scope で残す（同じ scope に書くジョブどうしが互いのキャッシュを上書きするため）。
  - 毎週の定期実行では、選んだ検査のステージだけキャッシュを使わない。キャッシュが当たると、新しく公開された
    脆弱性を拾えないため。
  - `check` ジョブは前のジョブの結果にかかわらず必ず動き、すべて成功していなければ失敗する。前のジョブの失敗で
    飛ばされたジョブは成功と数えられてしまうため。必須のチェックはこれだけ（[ADR 0011](../adr/0011-ci-checks-as-cached-build-stages.md)）。
  - 検査のコマンドは置かない。
- **関係**：ターゲットごとに埋めるのは、行列のジョブと、キャッシュを使わない検査（[ファイルの分類](../overview.md#files)）。
- **当てはめた原則・パターン**：[高凝集・疎結合](../../../platform/plugins/principles/docs/reference/principles.md#cohesion)：ruleset が知るのは
  `check` だけなので、ジョブを足しても ruleset は変えない。
