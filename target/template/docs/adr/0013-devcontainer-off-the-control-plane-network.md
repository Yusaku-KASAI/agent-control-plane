---
status: accepted
date: 2026-10-17
amends: ["../../../../docs/adr/app/0014-control-plane-as-a-server.md"]
---

# 0013. devcontainer を control-plane のネットワークに入れない

## Context

ターゲットの devcontainer は、人が devcontainer から `acp run` で依頼できるよう、control-plane と外部の Docker
ネットワーク `agent-control-plane-net` を共有し、`ACP_SERVER` で API を指していた（[app ADR 0014](../../../../docs/adr/app/0014-control-plane-as-a-server.md)）。ところが
devcontainer には API のトークン（`ACP_API_TOKEN`）を渡さない。渡すと、そのコンテナから GitHub の認証情報を持つ
エージェントを起動できてしまうため。その結果、ほかのターゲットの devcontainer から `acp run` は使えず、
ネットワークは使われていなかった。依頼は Slack と、ホストの `acp run` で足りている。

ただし control-plane を開発するこのリポジトリでは、control-plane の compose が読むリポジトリの直下の `.env` に
トークンがあり、devcontainer はリポジトリをマウントするので、中からトークンが読める。ここではネットワークを
共有していると、devcontainer の中（Claude Code を含む）から API に依頼できた。

一方、エージェントのコンテナはこのネットワークに入っていない。devcontainer とエージェントのコンテナは、同じ
イメージで動き（ターゲットの契約のイメージ）、同じ環境で検査し（ADR 0005）、外への通信の制限も同じものを置く
（ADR 0008）。ネットワークだけ devcontainer が多くを持っていた。

人とエージェントの環境は、なるべく同じ能力を持つようにしたい。ネットワークは、その差の一つだった。

## Decision

- どのターゲットの devcontainer も、エージェントのコンテナと同じく、control-plane のネットワークに入らず、
  `ACP_SERVER` も渡さない。control-plane そのものを開発するこのリポジトリも例外にしない。ホストの準備
  （`init-host.sh`）はネットワークを作らない。
- control-plane は、compose のプロジェクトの既定のネットワークを使う。外部のネットワークを手で作る手順をなくす。
- devcontainer から control-plane を試すときだけ、人がその場でネットワークにつなぎ、トークンを手で渡す。
- 将来、エージェントのコンテナに control-plane やサイドカーへの道を足すときは、devcontainer にも同じ仕組みで
  同じ道を渡す。

## Alternatives

- **すべてのターゲットで残す**：使われていない経路が残り、エージェントとの差も残る。ほかのターゲットでは、API を
  守るのはトークンなので（app ADR 0014）、外すのは隔離のためというより、差と使われない設定をなくすため。
- **control-plane を開発するターゲットの devcontainer だけ残す**：control-plane を試しやすいが、同じ control-plane
  のエージェントとの差が残り、ターゲットごとの部分が増える。このリポジトリでは、トークンを読める devcontainer から
  API への道も残る。試すのはまれなので、その場でつなげば足りる。
- **残して、devcontainer 向けの権限を絞ったトークンを作る**：依頼者の識別（control-plane の #24）が要る。
  エージェントに control-plane への道を渡すときに、同じ仕組みで devcontainer にも渡す。

## Consequences

- 今ある devcontainer は、作り直す（`docker compose up -d --force-recreate` など）まで `agent-control-plane-net` に
  つながったまま。control-plane も、作り直すと compose の既定のネットワーク（`agent-control-plane_default`）に移る。
  ほかのターゲットが追従し（#131）、どの devcontainer も control-plane も作り直したあとは、`agent-control-plane-net`
  は誰も使わないので、`docker network rm agent-control-plane-net` で消してよい。追従の前に消すと、追従していない
  ターゲットの devcontainer が、外部のネットワークがなく起動できない。
- target-template の `init-host.sh`（共通）と `.devcontainer/compose.yaml`（埋める）の共通の部分が変わるので、
  ほかのターゲットも追従する（#131）。
