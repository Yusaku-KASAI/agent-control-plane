# docs

このリポジトリ（control-plane）の文書の一覧。分け方は conventions の[文書の体系](../platform/plugins/conventions/docs/architecture/documentation.md)にある。
platform と target-template の文書は、それぞれ [platform/docs/README.md](../platform/docs/README.md) と
[target/template/docs/README.md](../target/template/docs/README.md) に、原則とパターン、共通の決まりの文書は
[principles](../platform/plugins/principles/docs/README.md) と [conventions](../platform/plugins/conventions/docs/README.md) の
`docs/README.md` にある。

| 文書 | 種類 | 何があるか |
| --- | --- | --- |
| [overview.md](overview.md) | 解説 | 狙い、考え方、全体の構成と地図、目指す姿 |
| [architecture/app/components.md](architecture/app/components.md) | 解説 | control-plane の構成：プロセス、部品、依頼の流れ |
| [architecture/app/requests.md](architecture/app/requests.md) | 解説 | 依頼と知らせ：依頼とターンの状態、Slack、HTTP API、状態メッセージ、再起動の知らせ |
| [architecture/app/sessions.md](architecture/app/sessions.md) | 解説 | セッション：ライフサイクル、記録、ロック、ワーカー、使用量の記録 |
| [architecture/app/cleanup.md](architecture/app/cleanup.md) | 解説 | 片付け：状態の軸と判定、作業領域と GitHub から読む事実 |
| [architecture/app/workspace.md](architecture/app/workspace.md) | 解説 | 作業領域とブランチ：clone、セッションのブランチ、git のフック |
| [architecture/app/credentials.md](architecture/app/credentials.md) | 解説 | 認証情報：Claude、GitHub App、Slack の発行と更新、セッションごとの置き場所 |
| [architecture/app/containers.md](architecture/app/containers.md) | 解説 | エージェントのコンテナ：Docker のプロキシ、実行、プラグインの読み込み |
| [architecture/codebase/layers.md](architecture/codebase/layers.md) | 解説 | Go のコードの層と依存の向き |
| [architecture/codebase/devcontainer.md](architecture/codebase/devcontainer.md) | 解説 | target-template の環境のファイルのうち、このリポジトリが埋める部分（Go のツールチェーンと検査） |
| [guides/operations.md](guides/operations.md) | 手順 | サーバーの準備と起動、Slack のトークンの取り直し、クライアントのビルド |
| [reference/usage.md](reference/usage.md) | 参照 | 依頼の仕方（Slack、CLI） |
| [reference/coding.md](reference/coding.md) | 参照 | Go の書き方 |
| [reference/issues.md](reference/issues.md) | 参照 | Issue のラベル |
| [acp.example.yaml](../acp.example.yaml) | 参照 | 設定の項目と既定値 |
| [.env.example](../.env.example) | 参照 | `docker compose` が読む値（API トークン、ホストのパス、UID と GID、Docker ソケットのグループ） |
| [deploy/slack-app-manifest.yaml](../deploy/slack-app-manifest.yaml) | 参照 | エージェントの Slack アプリのマニフェスト |
| [adr/app/](adr/app/)、[adr/codebase/](adr/codebase/) | 判断 | アプリとコードの判断 |
