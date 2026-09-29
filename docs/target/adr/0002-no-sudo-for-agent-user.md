# 0002. エージェントが動くユーザーに sudo を与えない

- Status: Accepted
- Date: 2026-09-30

## Context

エージェント（Claude Code）はコンテナ内の `node` ユーザーで動く。sudo があればパッケージを追加できて便利だが、
ファイアウォールや entrypoint などコンテナの前提も変えられてしまう。なお Claude Code の
`--dangerously-skip-permissions` が拒否されるのは root（uid 0）で動かした場合であり、`node` が sudo を
持つこと自体は影響しない。

## Decision

- `node` には sudo を与えない（手動でファイアウォールを再適用するための `init-firewall.sh` だけは例外）。
- apt のパッケージ一覧はイメージに残し、人間が `docker exec -u root` で入って一時的にインストールできるようにする。
- 恒常的に必要なツールは Dockerfile に追加する。

## Alternatives

- **apt だけ sudo を許可する**：パッケージのインストールスクリプトは root で動くため、実質的に root と同じになる。
- **sudo を全面的に許可する**：エージェント自身がファイアウォールなどの前提を外せてしまう。

## Consequences

- エージェントが必要とするツールが足りないときは、Dockerfile の変更として扱う。
