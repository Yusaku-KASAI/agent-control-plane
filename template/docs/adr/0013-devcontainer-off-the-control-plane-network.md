---
status: accepted
date: 2026-10-17
amends: ["../../../docs/adr/app/0014-control-plane-as-a-server.md"]
---

# 0013. ターゲットの devcontainer を control-plane のネットワークに入れない

## Context

ターゲットの devcontainer は、人が devcontainer から `acp run` で依頼できるよう、control-plane と外部の Docker
ネットワーク `agent-control-plane-net` を共有し、`ACP_SERVER` で API を指していた（[app ADR 0014](../../../docs/adr/app/0014-control-plane-as-a-server.md)）。ところが
devcontainer には API のトークン（`ACP_API_TOKEN`）を渡さない。渡すと、そのコンテナから GitHub の認証情報を持つ
エージェントを起動できてしまうため。その結果、devcontainer から `acp run` は使えず、ネットワークは使われていな
かった。依頼は Slack と、ホストの `acp run` で足りている。

## Decision

- ターゲットの devcontainer は、compose のプロジェクトの既定のネットワークだけに入り、control-plane のネットワークに
  入らない。`ACP_SERVER` も渡さない。ホストの準備（`init-host.sh`）はネットワークを作らない。
- control-plane そのものを開発するターゲットは、control-plane を試すために、自分の devcontainer をネットワークに
  入れてよい。これは `.devcontainer/compose.yaml` のターゲットごとの部分にし、ネットワークはそのターゲットが作る。
  トークンは渡さず、試すときに人が手で渡す。

## Alternatives

- **すべてのターゲットで残す**：使われていない経路で、devcontainer から control-plane の API に届く面を残す。
- **残して、devcontainer 向けの権限を絞ったトークンを作る**：依頼者の識別（control-plane の #24）が要り、今は
  devcontainer から依頼する使い方がない。
- **control-plane の devcontainer も外し、試すときに `docker network connect` でつなぐ**：共通のファイルに差が
  出ないが、control-plane を開発するときに毎回つなぐ手間がかかる。

## Consequences

- 今あるターゲットの devcontainer は、作り直す（`docker compose up -d --force-recreate` など）まで、ネットワークに
  つながったまま。作り直せば外れる。ネットワークそのものは control-plane が使い続けるので消さない。
- target-template の `init-host.sh`（共通）と `.devcontainer/compose.yaml`（埋める）が変わるので、ほかのターゲットも
  追従する。
