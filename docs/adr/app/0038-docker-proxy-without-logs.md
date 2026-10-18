---
status: accepted
date: 2026-10-18
amends: [15]
---

# 0038. Docker のプロキシはコンテナのログの API を通さず、エージェントの出力は attach で受ける

## Context

ADR 0015 は、Docker ソケットをプロキシに持たせ、control-plane が使う API だけを通すとした。その Context は、
control-plane が使う API に「ログ取得」を挙げていた。

実際の runner は、エージェントの出力（stream-json）をコンテナの attach で受け、ログの API は呼ばない。起動時に
残ったエージェントのコンテナを消すために、コンテナの一覧を呼ぶ。`compose.yaml` のプロキシの許可も、この使い方に
合わせてある（Issue #75）。

## Decision

- プロキシが通すのは、runner が呼ぶ API だけにする：`_ping` と `version`、コンテナの一覧、作成、attach、起動、
  終了待ち、削除。
- コンテナのログの API は通さない。エージェントの出力は attach で受ける。
- 許可の一覧の正は `compose.yaml` にする。runner が新しい API を呼ぶときは、同じ変更で許可を足す（ADR 0015 の
  Consequences のとおり）。

## Alternatives

- **ログの API も許可して、ADR 0015 の Context に合わせる**：使わない API を通すことになる。パスの正規表現では
  エージェントのコンテナに絞れないので、control-plane が乗っ取られたとき、devcontainer などほかのコンテナの出力まで
  読める。

## Consequences

- control-plane の権限は、使う操作だけに保たれる。
- 終わったコンテナの出力を control-plane から読み直す手段はない。調べるときは、ホストで `docker logs` を使う。
