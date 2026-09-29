# Devcontainer

Anthropic の [Claude Code devcontainer](https://github.com/anthropics/claude-code/tree/main/.devcontainer)
をベースにした開発・エージェント実行環境。

## 初回セットアップ（ホストで一度だけ）

```bash
docker network create agent-control-plane-net
docker volume create agent-control-plane_claude-code-bashhistory
docker volume create agent-control-plane_claude-code-config
```

## 基本ポリシー

- コンテナの振る舞いは Dockerfile / compose.yaml / entrypoint で決める。devcontainer.json は VS Code 側の
  配線だけを持つ（[ADR 0001](../docs/target/adr/0001-devcontainer-independent-of-vscode.md)）。
- `base` ステージには、リポジトリを問わずエージェントが使うもの（Node・Python/uv・シェルツール・
  claude-code・ファイアウォール等）だけを入れる。リポジトリ固有のツールチェーンは後段のステージに置く。

## 構成

- イメージは `base` → `go`（このリポジトリのツールチェーン）の 2 段。
- `..` を `/workspace/agent-control-plane` に読み書き可、`../..` を `/workspace` に**読み取り専用**で
  マウントする。隣に置いたリポジトリは参照できるが書き込めない。
- ファイアウォールは `FIREWALL_ENABLED=false`（compose.yaml）で無効。有効にする場合は
  `init-firewall.sh` の allowlist を必要に応じて更新する。

## tmux とターミナルのベル

hook プロセスは制御端末を持たないため、hook からベルを届ける経路は `tmux list-clients` しかない。
既定のターミナルプロファイルが tmux なのはこのため。

## VS Code の設定

エディタ設定は `devcontainer.json` の `customizations.vscode.settings`（コンテナに attach したウィンドウ）と
`.vscode/settings.json`（attach していないウィンドウ）に同一内容で置く。ベル設定のように片方だけだと
効いたり効かなかったりするものがあるため。
