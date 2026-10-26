# Claude Code の設定

ターゲットのリポジトリに置く Claude Code の設定（`.claude/settings.json`）。

## 構成

一つのファイルが、二つの使われ方の設定を持つ。プラグインの宣言、回答の言語（`language`）、worktree の作り方
（`worktree`）は、control-plane 経由でも devcontainer で直接でも読まれる。`permissions` は人が対話で使うときのためのもので、control-plane 経由では
権限の確認なしで動く（[ハーネスの契約](../../../platform/docs/reference/harness.md "在りか")）。

## 構成要素

### `.claude/settings.json`

- **責務**：使うプラグインの宣言、回答の言語、人が対話で使うときの Claude Code の設定を持つ。
- **置くもの / 置かないもの**：
  - プラグインは `extraKnownMarketplaces` と `enabledPlugins` で宣言する。
  - `language` は回答の言語。決まりは CLAUDE.md の言語の節にあり、ここはそれを Claude Code に渡す。
  - `worktree.baseRef` は `head`。`isolation: worktree` で動くサブエージェント（roles の reviewer、guide、pm）の worktree を、
    既定（`fresh`、origin の既定のブランチ）ではなく今の HEAD から作り、まだ push していないコミットも見えるようにする。
    その worktree は `.claude/worktrees/` の下にでき、変更があると残るので、`.gitignore` で ignore する
    （[platform の ADR 0016](../../../platform/docs/adr/0016-subagents-leave-the-work-area.md)）。
  - `permissions`（既定のモードと、確認せずに使えるツールの一覧）は、人が対話で使うときの設定。対話での
    使い方は[手順](../guides/claude-code.md#permissions "在りか")にある。
- **関係**：プラグインの宣言、`language`、`worktree.baseRef` は[ターゲットの契約](../../../platform/docs/reference/target.md#plugins)の一部で、control-plane も
  エージェントもこれを読む（[ADR 0004](../adr/0004-plugins-declared-in-settings.md)）。
- **当てはめた原則・パターン**：[スキーマを先に](../../../platform/plugins/principles/docs/reference/principles.md#schema-first)：プラグインの
  宣言の形と置き場所はターゲットの契約が先に決め、このファイルはそれに合わせる。
