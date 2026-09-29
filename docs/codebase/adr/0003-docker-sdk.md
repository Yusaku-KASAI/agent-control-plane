# 0003. Docker の操作には moby/moby/client を使う

- Status: Accepted
- Date: 2026-09-30

## Context

control-plane はエージェントのコンテナを作成・起動・待機・削除する（app ADR 0004）。Go から Docker を
操作する方法には、公式の Go SDK と、docker コマンドの実行がある。公式 SDK は `github.com/docker/docker`
から `github.com/moby/moby/client` に移っている。

## Decision

`github.com/moby/moby/client` を使う。使う API は adapter 内で小さなインターフェースに絞る。

## Alternatives

- **`github.com/docker/docker`**：更新が止まっている。
- **docker コマンドを `os/exec` で実行する**：control-plane の実行環境に docker コマンドが必要になり、
  出力の解析も壊れやすい。

## Consequences

- moby/moby/client はまだ 1.0 前で、API が変わりうる。影響は `internal/adapter/docker` に閉じる。
