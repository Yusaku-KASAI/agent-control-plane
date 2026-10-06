# 0007. 隣のリポジトリのマーケットプレイスは devcontainer でのパスで宣言し、control-plane がそこだけマウントする

- Status: Accepted
- Date: 2026-10-06

## Context

control-plane は、ターゲットの中にあるマーケットプレイスのプラグインだけを `--plugin-dir` で読み込ませていた
（ADR 0004）。共通のロールやプラグインはこのリポジトリの `platform/` にあり、他のターゲットの中にはない。
headless の Claude Code は宣言されたプラグインをインストールしないので、他のターゲットのエージェントは roles
などを使えない。devcontainer は隣のリポジトリを `/workspace` に読み取り専用でマウントするので、人が対話で
使うときは `/workspace/agent-control-plane/platform` を `directory` のマーケットプレイスとして宣言すれば届く。

## Decision

- 隣のリポジトリにあるマーケットプレイスは、`.claude/settings.json` に devcontainer でのパス
  （`/workspace/<リポジトリ>/...`）で宣言する。
- control-plane は、そのパスをホストでターゲットの隣にあるディレクトリに読み替え、そのディレクトリだけを
  エージェントのコンテナの同じパスに読み取り専用でマウントし、プラグインを `--plugin-dir` で渡す。
- ターゲット自身の中を指す絶対パスや、`/workspace` の外を指すパスは読み替えない。

## Alternatives

- **control-plane の設定でマーケットプレイスの名前とディレクトリを対応させる**：人向けの宣言
  （`settings.json`）とエージェント向けの設定の二か所に書くことになる。
- **隣のリポジトリをすべてマウントする**：関係のないリポジトリまでエージェントに見える。
- **control-plane が GitHub などからマーケットプレイスを取得する**：取得のための認証と版の管理が要る。
  ホストに隣のリポジトリがない場合（リモートから取得するターゲットなど）に改めて考える。

## Consequences

- 人とエージェントが、同じ宣言から同じマーケットプレイスを使う。control-plane に設定は要らない。
- ターゲットの隣にマーケットプレイスのリポジトリを clone しておく必要がある。そのディレクトリの今の内容が
  そのまま使われ、コミットしていない変更も届く。
- devcontainer が隣のリポジトリを `/workspace` に置くという決まりに頼る。
