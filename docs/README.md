# docs

このリポジトリ（control-plane）の文書の一覧。分け方は platform の[文書の体系](../platform/docs/architecture/documentation.md)にある。
platform と target-template の文書は、それぞれ [platform/docs/README.md](../platform/docs/README.md) と
[template/docs/README.md](../template/docs/README.md) にある。

| 文書 | 種類 | 何があるか |
| --- | --- | --- |
| [overview.md](overview.md) | 解説 | 狙い、考え方、全体の構成（3 つの区切りの関係）、目指す姿 |
| [architecture/app/sessions.md](architecture/app/sessions.md) | 解説 | セッションのライフサイクルと、属するものの置き場所 |
| [architecture/codebase.md](architecture/codebase/layers.md) | 解説 | Go のコードの層と依存の向き |
| [guides/operations.md](guides/operations.md) | 手順 | サーバーの準備と起動、Slack のトークンの取り直し、クライアントのビルド |
| [reference/usage.md](reference/usage.md) | 参照 | 依頼の仕方（Slack、CLI） |
| [reference/coding.md](reference/coding.md) | 参照 | Go の書き方 |
| [acp.example.yaml](../acp.example.yaml) | 参照 | 設定の項目と既定値 |
| [adr/app/](adr/app)、[adr/codebase/](adr/codebase) | 判断 | アプリとコードの判断 |

Issue のラベル（どこを変えるか）は、区切りと関心に合わせて `app`、`codebase`、`target`（target-template）、`platform`。
