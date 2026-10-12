# platform の全体像

platform は、ターゲットをまたいで使うエージェントの能力（ロール、スキル、hook）を、Claude Code のプラグインの
マーケットプレイス `acp-platform` としてまとめたもの（[ADR 0001](adr/0001-platform-as-plugin-marketplace.md)）。マーケットプレイスの中のものはすべての
エージェントに届くので、ハーネスの契約とエージェントの用語もここに置く。

platform は principles と conventions だけを前提にし、エージェントを動かす側（control-plane と devcontainer）とは
[ハーネスの契約](reference/harness.md)だけでつながる。区切りの分け方と依存の向きの決まりは、conventions の
[文書の体系](../plugins/conventions/docs/architecture/documentation.md)にある。

## 構成

```
platform/
  .claude-plugin/marketplace.json   マーケットプレイス acp-platform
  plugins/
    harness/ roles/ github/ …       エージェントの能力（platform の区切り。architecture/plugins.md）
    principles/                     どこでも通用する原則とパターン（別の区切り）
    conventions/                    このプロジェクト群の決まり（別の区切り）
  docs/                             platform 自身の文書（README.md が一覧）
```

`platform/` というディレクトリは、platform という区切りであると同時に、principles と conventions という別の区切りを
エージェントに届ける入れ物も兼ねる。ほかのターゲットのエージェントに届くのはマーケットプレイスの中だけなので、
すべての区切りが前提にする principles と conventions もここにプラグインとして置く。この二つは platform に属さず、
platform を前提にしない。

`docs/` に書くのは platform 自身のことだけ。各プラグインの中の構成は、そのプラグインの `docs/` に書く。原則と
このプロジェクト群の決まりは、principles と conventions の `docs/` にある。

## 届き方

- ターゲットは `.claude/settings.json` で `acp-platform` を宣言し、使うプラグインを有効にする。
- control-plane から起動するエージェントには、control-plane が宣言を読んで `--plugin-dir` で読み込ませる。プラグインに
  渡す認証情報や環境は、[ハーネスの契約](reference/harness.md)にある。
- 人が対話で使うときは、[対話で使う準備](guides/use-interactively.md)のとおりに入れる。

## プラグイン

各プラグインの責務と、プラグインの間の関係は [architecture/plugins.md](architecture/plugins.md) にある。
プラグインには、どのターゲットでも通用する内容だけを書く。プラグインの検査（`claude plugin validate ./platform` と、
`bin/` と `scripts/` の `shellcheck`）は `make check-platform` で行う。
