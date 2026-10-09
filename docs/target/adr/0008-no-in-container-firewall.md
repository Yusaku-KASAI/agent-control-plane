# 0008. コンテナの中にファイアウォールを置かない

- Status: Accepted
- Date: 2026-10-07

## Context

devcontainer には、Claude Code の公式の devcontainer にならった、外への通信を許可リストで絞る iptables の
ファイアウォール（`init-firewall.sh`）と、その記録のための tcpdump や rsyslog、`node` が sudo で再適用するための
例外（ADR 0002）が入っていた。実際には devcontainer でもエージェントのコンテナでも無効にしていた。エージェントの
コンテナには `NET_ADMIN` を与えていないので、有効にもできない。エージェントにはコンテナの中で何をしてもよいと
しているので（app ADR 0003）、コンテナの中で組んだ制限はエージェント自身が外せる前提で考える必要がある。

## Decision

- コンテナの中のファイアウォールと、そのためのツール、権限（`NET_ADMIN` などの capability、`/dev/net/tun`）、
  sudo の例外、記録の仕組みをなくす。`node` には sudo を一切与えない。
- エージェントの外への通信を絞る必要があれば、コンテナの外（Docker のネットワークやプロキシ）で行う。

## Alternatives

- **devcontainer で人が使うときのために残す**：使っておらず、イメージ、起動時の処理、権限、ドキュメントを
  増やしている。外への通信を絞る仕組みは、人とエージェントで同じものをコンテナの外に置くほうがよい。
- **エージェントのコンテナに `NET_ADMIN` を与えて有効にする**：コンテナの中で何でもできるエージェントに、
  ネットワークの設定を変える権限まで渡すことになる。

## Consequences

- イメージから iptables、ipset、dnsutils、aggregate、tcpdump、rsyslog、sudo などが消え、entrypoint の処理が減る。
- 今は外への通信を絞っていない。
