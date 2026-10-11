# CI

PR と main への push ごとに、devcontainer のイメージの中で `make check` を実行する
（[ADR 0005](../adr/0005-ci-runs-in-devcontainer-image.md)）。あわせて control-plane のイメージがビルドできることを
確かめる（`check-image`）。

- `make check` は関心ごとの検査（`check-go`、`check-platform`、`check-target`、`check-docs`）をまとめたもの
  （[ADR 0011](../adr/0011-ci-checks-as-cached-build-stages.md)）。検査が読むファイルを増やしたら、Dockerfile の
  そのステージのコピーにも足す。
- CI は関心ごとのジョブ（`check-go`、`check-platform`、`check-target`、`check-docs`）で、Dockerfile の同じ名前のステージを、
  イメージを書き出さずにビルドする（`check-image` も書き出さない）。各ステージはその検査が読むファイルだけを
  コピーして実行するので、それらが変わらなければ層のキャッシュが当たり、検査は実行されない。層は GitHub Actions の
  キャッシュにジョブごとに残す。
- ジョブを足しても ruleset は変えない（必須のチェックは `check` だけ。[契約](../reference/contract.md#checks)）。
- キャッシュが当たると新しく公開された脆弱性を拾えないので、週に1回、`check-go` だけキャッシュを使わずに実行する。
