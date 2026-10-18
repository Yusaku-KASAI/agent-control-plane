---
status: accepted
date: 2026-10-18
amends: [15, 17]
---

# 0038. Docker のプロキシはコンテナのログと停止の API を通さず、出力は attach で受け、強制の削除で止める

## Context

ADR 0015 は、Docker ソケットをプロキシに持たせ、control-plane が使う API だけを通すとした。その Context は、
control-plane が使う API に「ログ取得」を挙げていた。

実際の runner は、エージェントの出力（stream-json）をコンテナの attach で受け、ログの API は呼ばない。起動時に
残ったエージェントのコンテナを消すために、コンテナの一覧を呼ぶ。`compose.yaml` のプロキシの許可も、この使い方に
合わせてある（Issue #75）。

ADR 0017 の Consequences は、プロキシで「コンテナの標準入出力への接続と停止の API」を許可するとした。実際の
runner はコンテナを止めるのに停止の API を使わず、削除（強制）で止める。

## Decision

- プロキシが通すのは、runner が呼ぶ API だけにする：`_ping` と `version`、コンテナの一覧、作成、attach、起動、
  終了待ち、削除。
- コンテナのログの API と停止の API は通さない。エージェントの出力は attach で受け、コンテナは強制の削除で
  止める。
- 許可の一覧の正は `compose.yaml` にする。runner が新しい API を呼ぶときは、同じ変更で許可を足す（ADR 0015 の
  Consequences のとおり）。

## Alternatives

- **ログの API と停止の API も許可して、ADR 0015 と 0017 に合わせる**：使わない API を通すことになり、最小権限に
  反する。

## Consequences

- control-plane の権限は、使う操作の種類だけに保たれる。
- 許可はパスの正規表現で書くので、どのコンテナに対する操作かは絞れない。control-plane が乗っ取られると、attach で
  ほかのコンテナ（devcontainer など）の出力を読み、標準入力に書ける（attach は過去の出力も流せる）。ADR 0015 の
  Consequences の、中身を検査しない弱さと同じく残る。
- エージェントのプロセスが終わると、control-plane はコンテナをすぐ消すので、終わったあとの出力を読み直す手段は
  ない。異常で終わったときは、標準エラーの末尾をエラーに含める。動いている間は、ホストで `docker logs` を使える。
