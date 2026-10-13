# Claude Code の設定

ターゲットのリポジトリに置く Claude Code の設定（`.claude/settings.json`）。

## 構成

一つのファイルが、二つの使われ方の設定を持つ。プラグインの宣言と回答の言語（`language`）は、control-plane 経由でも
devcontainer で直接でも読まれる。`permissions` は人が対話で使うときのためのもので、control-plane 経由では権限の確認なしで動く
（[ハーネスの契約](../../../platform/docs/reference/harness.md "案内")）。

## 構成要素

### `.claude/settings.json`

- **責務**：使うプラグインの宣言、回答の言語、人が対話で使うときの Claude Code の設定を持つ。
- **置くもの / 置かないもの**：
  - プラグインは `extraKnownMarketplaces` と `enabledPlugins` で宣言する。
  - `language` は回答の言語。決まりは CLAUDE.md の言語の節にあり、ここはそれを Claude Code に渡す。
  - `permissions`（既定のモードと、確認せずに使えるツールの一覧）は、人が対話で使うときの設定。対話での
    使い方は devcontainer の[手順](../guides/devcontainer.md#permissions "案内")にある。
- **関係**：プラグインの宣言は[ターゲットの契約](../reference/contract.md#plugins)の一部で、control-plane も
  エージェントもこれを読む（[ADR 0004](../adr/0004-plugins-declared-in-settings.md)）。
- **当てはめた原則・パターン**：[スキーマを先に](../../../platform/plugins/principles/docs/reference/principles.md#schema-first)：プラグインの
  宣言の形と置き場所はターゲットの契約が先に決め、このファイルはそれに合わせる。
