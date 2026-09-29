# 0004. Docker の操作は control-plane だけが行う

- Status: Accepted
- Date: 2026-09-30

## Context

エージェントのコンテナ、サイドカーの起動・停止には Docker の操作が必要になる。Docker ソケットを持つことは、
ホストの root 権限を持つことに等しい。

## Decision

- エージェントのコンテナには Docker ソケットを渡さない。特権モードも使わない。
- コンテナの操作は control-plane だけが行う。control-plane 自体もコンテナとして動き、決まった操作だけを
  行えるよう、使える Docker API を限定する。

## Alternatives

- **エージェントのコンテナに Docker ソケットを渡す**：隔離が無意味になる（ADR 0003）。

## Consequences

- control-plane が信頼の境界になる。control-plane の Docker 操作はエージェントの入力から直接組み立てない。
- エージェントが Docker を必要とする作業（統合テストなど）は、control-plane が用意する範囲か CI で行う。
