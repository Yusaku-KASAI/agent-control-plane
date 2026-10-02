# platform

`platform/` は、ターゲットをまたいで使うエージェントの能力を Claude Code のプラグインとしてまとめたもの。
将来 agent-platform として別のリポジトリに切り出す（[ADR 0001](adr/0001-platform-as-plugin-marketplace.md)）。

## 構成

```
platform/
  .claude-plugin/marketplace.json   マーケットプレイス acp-platform
  plugins/
    roles/                          共通のロール（agents/<ロール名>.md）
```

## 使われ方

- ターゲットは `.claude/settings.json` で使うプラグインを宣言する（[target ADR 0004](../target/adr/0004-plugins-declared-in-settings.md)）。
- control-plane から起動するエージェントには、control-plane が宣言を読んで `--plugin-dir` で読み込ませる。
- 人間が対話で使うときは、最初に一度マーケットプレイスを登録してプラグインを入れる。

  ```bash
  claude plugin marketplace add ./platform
  claude plugin install roles@acp-platform
  ```

- プラグインを変えたら `claude plugin validate ./platform` で検査する。
- プラグインには、どのターゲットでも通用する内容だけを書く。
