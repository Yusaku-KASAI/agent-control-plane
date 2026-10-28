# platform の全体像

platform は、ターゲットをまたいで使うエージェントの能力（ロール、スキル、hook）を、Claude Code のプラグインとして
まとめたもの。プラグインはマーケットプレイス `acp-platform` に入れて配る（[ADR 0001](adr/0001-platform-as-plugin-marketplace.md)）。

platform は principles、conventions と契約（[エージェントの用語集](reference/glossary.md)、[ハーネスの契約](reference/harness.md)、
[ターゲットの契約](reference/target.md)）を前提にし、エージェントを動かす側（control-plane と devcontainer）とターゲットとは、
契約だけでつながる。区切りの分け方と
依存の向きの決まりは、conventions の[文書の体系](../plugins/conventions/docs/architecture/documentation.md)にある。

## 構成

```
platform/
  .claude-plugin/marketplace.json   マーケットプレイス acp-platform
  plugins/
    harness/ roles/ github/ …       エージェントの能力（platform の中の項目。architecture/plugins.md）
    principles/                     どこでも通用する原則とパターン（別の区切り）
    conventions/                    このプロジェクト群の決まり（別の区切り）
  docs/                             platform の文書（README.md が一覧）と、契約の文書の置き場所
```

`platform/` というディレクトリは、platform の置き場所であると同時に、マーケットプレイスとしてほかの区切りも運ぶ
入れ物を兼ねる（何を運ぶかは[用語集](reference/glossary.md "在りか")のマーケットプレイス）。運ばれるほかの区切りは
platform に属さない。

`docs/` には platform の文書を置く。置き場所として、契約という項目に属するエージェントの用語集、ハーネスの契約、
ターゲットの契約もここに置く。各プラグインの中の構成は、そのプラグインの `docs/` に書く。

## 届き方

- ターゲットは `.claude/settings.json` で `acp-platform` を宣言し、使うプラグインを有効にする。
- control-plane から起動するエージェントには、control-plane が宣言を読んで `--plugin-dir` で読み込ませる。プラグインに
  渡す認証情報や環境は、[ハーネスの契約](reference/harness.md)にある。
- 人が対話で使うときは、[対話で使う準備](guides/use-interactively.md)のとおりに入れる。

## プラグイン

各プラグインの責務と、プラグインの間の関係は [architecture/plugins.md](architecture/plugins.md) にある。
プラグインには、どのターゲットでも通用する内容だけを書く。プラグインの形は `claude plugin validate` で、`bin/` と
`scripts/` のシェルスクリプトは `shellcheck` で確かめる。

<a id="map-platform"></a>

## 地図

platform の中の項目と前提の向き（[形式](../plugins/conventions/docs/reference/formats.md#map)）。

| 項目 | 親 | 置き場所 | 前提にする |
| --- | --- | --- | --- |
| harness | platform | `../plugins/harness/` | |
| github | platform | `../plugins/github/` | |
| slack | platform | `../plugins/slack/` | |
| roles | platform | `../plugins/roles/` | harness, github |
| bell | platform | `../plugins/bell/` | |
