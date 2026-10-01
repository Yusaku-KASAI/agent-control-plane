# 0015. Docker ソケットはプロキシに持たせ、使う API だけを通す

- Status: Accepted
- Date: 2026-10-01

## Context

control-plane が使える Docker API を限定する（ADR 0004）。Docker ソケットを持つことはホストの root 権限を
持つことに等しい。control-plane が使うのは、コンテナの作成・起動・終了待ち・ログ取得・削除と、API の
バージョン確認だけである。

## Decision

- Docker ソケットは `wollomatic/socket-proxy` のコンテナだけに渡す。control-plane はこのプロキシに
  専用の内部ネットワーク越しに接続する。
- プロキシは、HTTP メソッドごとにパスの正規表現で許可した API だけを通す。

## Alternatives

- **API の種類単位で許可するプロキシ（docker-socket-proxy など）**：コンテナ系を許可すると、exec や
  コンテナへのファイル書き込みまで通る。
- **作成するコンテナの中身まで検査する自前のプロキシ**：特権モードやホストのルートのマウントも防げるが、
  作る量が大きい。
- **control-plane にソケットを直接渡す**：control-plane が乗っ取られたときにホスト全体を操作される。

## Consequences

- コンテナ作成のリクエストの中身（特権モード、マウント先、イメージ）は検査していない。control-plane が
  乗っ取られた場合の備えとしては不十分で、中身を検査する仕組みは別に要る。
- runner が新しい Docker API を使うときは、プロキシの許可も追加する必要がある。
