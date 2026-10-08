# 0001. コンテナの振る舞いを VS Code から独立させる

- Status: Accepted, Amended by 0009
- Date: 2026-09-30

## Context

devcontainer.json には、ユーザーの UID 再マップ、features、ライフサイクルコマンド、マウントなど、
コンテナ自体の振る舞いを変える機能が多い。これらは VS Code（Dev Containers）経由で起動したときにしか
効かないため、`docker compose` で直接起動した場合と挙動がずれる。VS Code を使わずに同じイメージを
起動する場面（エージェントの実行など）もある。

## Decision

- コンテナの振る舞い（イメージ、マウント、環境変数、権限、起動時処理）は Dockerfile と compose.yaml に
  置く。devcontainer.json は VS Code 側の配線（拡張機能、エディタ設定、`remoteUser`、権限不要な
  `postAttachCommand`）に限る。
- UID/GID の整合は `docker-entrypoint.sh` だけが担う。`updateRemoteUserUID` は `false` にする。
- `shutdownAction: none` とし、VS Code のウィンドウを閉じてもコンテナは止めない。

## Alternatives

- **`updateRemoteUserUID`（VS Code の UID 再マップ）に任せる**：VS Code 起動時にしか効かない。さらに
  entrypoint が chown した後に UID を変えるため、ボリュームの所有者がずれる。
- **ビルド時の UID 引数**：ホストごとに再ビルドが必要になる。
- **devcontainer features / `postCreateCommand` での sudo セットアップ**：VS Code 経由でしか再現しない。

## Consequences

- `docker compose -f .devcontainer/compose.yaml up` で、VS Code 経由と同じコンテナが得られる。
- コンテナの挙動を変えるときは devcontainer.json ではなく Dockerfile / compose.yaml / entrypoint を触る。
