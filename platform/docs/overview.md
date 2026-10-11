# platform の全体像

platform は、ターゲットをまたいで使うエージェントの能力を、Claude Code のプラグインのマーケットプレイス
`acp-platform` としてまとめたもの。将来 agent-platform として別のリポジトリに切り出す
（[ADR 0001](adr/0001-platform-as-plugin-marketplace.md)）。マーケットプレイスの中のものはすべてのエージェントに届くので、
ハーネスの契約とエージェントの用語もここに置く。どこでも通用する原則とパターン（principles）と、このプロジェクト群の
決まり（conventions）も、それぞれプラグインとしてここから配る。

## 構成

```
platform/
  .claude-plugin/marketplace.json   マーケットプレイス acp-platform
  plugins/                          プラグイン（architecture/plugins.md）
  docs/                             すべてのエージェントに届く文書（README.md が一覧）
```

## 届き方

- ターゲットは `.claude/settings.json` で `acp-platform` を宣言し、使うプラグインを有効にする。
- control-plane から起動するエージェントには、control-plane が宣言を読んで `--plugin-dir` で読み込ませる。プラグインに
  渡す認証情報や環境は、[ハーネスの契約](reference/harness.md)にある。
- 人が対話で使うときは、[対話で使う準備](guides/use-interactively.md)のとおりに入れる。

## プラグインの間の関係

```
roles ──前提にする──▶ harness（動き方と契約の在りか）
  │                 conventions（決まり、設計の観点）──前提にする──▶ principles（原則とパターン）
  └─使う──▶ github、slack のスキル
bell（対話のときのベル）は独立
```

各プラグインの説明は [architecture/plugins.md](architecture/plugins.md) にある。
プラグインには、どのターゲットでも通用する内容だけを書く。プラグインの検査（`claude plugin validate ./platform` と、
`bin/` と `scripts/` の `shellcheck`）は `make check-platform` で行う。
