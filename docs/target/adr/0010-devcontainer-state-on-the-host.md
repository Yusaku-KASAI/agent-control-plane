# 0010. devcontainer のログインや履歴は、ホストのディレクトリに置いてマウントする

- Status: Accepted
- Date: 2026-10-08

## Context

devcontainer の Claude Code の設定（ログイン、会話の履歴、メモリ）とシェルの履歴は、名前を固定した external の
名前付きボリュームに置いていた。external にしたのは、`docker compose down -v` で消えず、compose のプロジェクト名が
変わっても同じボリュームを使うため。それでも、Docker の掃除（`docker volume prune -a` など）では消える。`gh` の設定は
通常の名前付きボリュームで、`down -v` でも消えていた。

## Decision

- Claude Code の設定、`gh` の設定、シェルの履歴は、ホストのディレクトリ（`DEV_STATE_DIR`、既定は
  `~/.local/state/devcontainers/<リポジトリ名>`）を bind mount する。リポジトリを並べたディレクトリの外に置く。
- 消えても取り直せるキャッシュ（Go のモジュールとビルドのキャッシュ）は、通常の名前付きボリュームに置く。
- ディレクトリは、ホストの準備のスクリプト（`init-host.sh`）が作る。devcontainer の起動の前に、人がコマンドで
  実行する。VS Code の `initializeCommand` には入れない。VS Code の操作がコンテナのライフサイクルに関わらない
  ようにするため（ADR 0001）。

## Alternatives

- **external の名前付きボリュームのまま**：Docker の掃除で消える。
- **リポジトリの中（gitignore）に置く**：隣のリポジトリとして、他のターゲットの devcontainer から読み取れる。

## Consequences

- Docker をどう掃除しても、ログインと履歴は残る。消すときは `DEV_STATE_DIR` を消す。
- 以前の external のボリュームの中身は、一度だけ `DEV_STATE_DIR` に写す。
- コンテナは `HOST_UID` / `HOST_GID` で動くので、ホストのユーザーの持ち物のディレクトリにそのまま書き込める。
- devcontainer を起動する前に `init-host.sh` を実行しておく必要がある（ADR 0001 の Consequences を変える）。
  実行しないと、状態のディレクトリを Docker が root の持ち物として作る。
