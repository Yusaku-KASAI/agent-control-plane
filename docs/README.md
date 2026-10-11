# docs

このリポジトリ（control-plane）の文書の一覧。分け方は conventions の[文書の体系](../platform/plugins/conventions/docs/architecture/documentation.md)にある。
platform と target-template の文書は、それぞれ [platform/docs/README.md](../platform/docs/README.md) と
[template/docs/README.md](../template/docs/README.md) にある。

| 文書 | 種類 | 何があるか |
| --- | --- | --- |
| [overview.md](overview.md) | 解説 | 狙い、考え方、全体の構成（3 つの区切りの関係）、目指す姿 |
| [architecture/app/components.md](architecture/app/components.md) | 解説 | control-plane の構成：プロセス、部品、依頼の流れ |
| [architecture/app/requests.md](architecture/app/requests.md) | 解説 | 依頼と知らせ：Slack、HTTP API、状態メッセージ、再起動の知らせ |
| [architecture/app/sessions.md](architecture/app/sessions.md) | 解説 | セッション：ライフサイクル、記録、ロック、ワーカー、片付け |
| [architecture/app/workspace.md](architecture/app/workspace.md) | 解説 | 作業領域とブランチ：clone、セッションのブランチ、共著者、片付けのための事実 |
| [architecture/app/credentials.md](architecture/app/credentials.md) | 解説 | 認証情報：Claude、GitHub App、Slack の発行と更新、セッションごとの置き場所 |
| [architecture/app/containers.md](architecture/app/containers.md) | 解説 | エージェントのコンテナ：Docker のプロキシ、実行、プラグインの読み込み |
| [architecture/codebase/layers.md](architecture/codebase/layers.md) | 解説 | Go のコードの層と依存の向き |
| [guides/operations.md](guides/operations.md) | 手順 | サーバーの準備と起動、Slack のトークンの取り直し、クライアントのビルド |
| [reference/usage.md](reference/usage.md) | 参照 | 依頼の仕方（Slack、CLI） |
| [reference/coding.md](reference/coding.md) | 参照 | Go の書き方 |
| [acp.example.yaml](../acp.example.yaml) | 参照 | 設定の項目と既定値 |
| [adr/app/](adr/app/)、[adr/codebase/](adr/codebase/) | 判断 | アプリとコードの判断 |

Issue のラベル（どこを変えるか）は、区切りと関心に合わせて `app`、`codebase`、`target`（target-template）、`platform`。
