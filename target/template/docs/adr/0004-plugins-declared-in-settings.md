---
status: accepted
date: 2026-10-03
amended_by: [7]
---

# 0004. ターゲットはプラグインを .claude/settings.json で宣言し、control-plane は --plugin-dir で読み込ませる

## Context

ターゲットは自分の上で動くエージェントの拡張を宣言する（app ADR 0001）。Claude Code の標準の宣言は
`.claude/settings.json` の `extraKnownMarketplaces` と `enabledPlugins` だが、宣言されたプラグインが入るのは
対話でフォルダを信頼したときで、`claude -p` の非対話の起動ではインストールされない。`--plugin-dir` で渡した
プラグインは、そのセッションだけ読み込まれる。

## Decision

- ターゲットが使うプラグインは `.claude/settings.json` で宣言する。ターゲットの中にあるマーケットプレイスは
  `directory` の相対パスで書く。
- control-plane は、ターゲットの中にあるマーケットプレイスから有効なプラグインを探し、`--plugin-dir` で渡す。
  それ以外のマーケットプレイスのプラグインは Claude Code に任せる。

## Alternatives

- **起動前に `claude plugin install` を実行する**：インストールは Claude Code の設定ディレクトリに記録され、
  そのディレクトリを複数のセッションで共有しているため、セッション同士で影響し合う。
- **ターゲットが別の独自ファイルでプラグインを宣言する**：人間が対話で使うときの標準の宣言と二重になる。
- **control-plane の設定ファイルでプラグインを指定する**：ターゲットが宣言するという方針に反する。

## Consequences

- control-plane から起動したエージェントと対話で使う人間が、同じ宣言から同じプラグインを使う。
- 人間は、最初に一度マーケットプレイスを登録してプラグインを入れる必要がある。
- GitHub などにあるマーケットプレイスのプラグインをエージェントに使わせるには、別の仕組みが要る。
