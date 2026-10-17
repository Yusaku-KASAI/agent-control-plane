---
status: accepted
date: 2026-10-01
amended_by: ["../../../template/docs/adr/0013-devcontainer-off-the-control-plane-network.md"]
---

# 0014. control-plane は常駐するサーバーとしてコンテナで動き、CLI はそのクライアントになる

## Context

Docker の操作は control-plane だけが行い、control-plane 自体もコンテナで動く（ADR 0004）。Slack など
外部からの依頼を受けるにも常駐するプロセスが要る。CLI が直接 Docker を操作する形では、dev container の
中から依頼できない。

## Decision

- `acp serve` が HTTP API（`POST /v1/turns`）で依頼を受け、ユースケースを実行する。`acp run` はこの API の
  クライアントで、Docker には触れない。CLI が直接実行する形はなくす。
- API はリクエストごとに共有のトークン（`ACP_API_TOKEN`）を Bearer で要求する。
- サーバーはリポジトリ直下の `compose.yaml` でコンテナとして動かす。設定ファイルが指すホストのパス
  （ターゲット、`sessions.root`、秘密鍵）は、コンテナにホストと同じパスでマウントする。
- ホストからは `127.0.0.1:8787`、dev container からは共有ネットワーク上の `control-plane:8787` で届く。
  dev container には API トークンを渡さない。

## Alternatives

- **CLI が直接実行する形を残す**：同じ処理への入口が 2 つになり、ロックや認証情報の扱いが分かれる。
  コンテナなしで試すときは、ホストで `acp serve` を動かせば済む。
- **パスをコンテナ内のパスに読み替える**：エージェントのコンテナのマウントはホストの Docker が解決するため、
  結局ホストのパスが要り、読み替えの対応表を持つことになる。
- **API に認証を付けない**：共有ネットワーク上の他のコンテナが、GitHub の認証情報を持つエージェントを
  動かせてしまう。

## Consequences

- 1 ターンの間、HTTP のリクエストはつながったままになる。クライアントが切断すると、そのターンは中断される。
- 秘密鍵の置き場所は、dev container から読める親ディレクトリの外にする。
