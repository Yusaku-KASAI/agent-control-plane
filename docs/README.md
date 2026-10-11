# docs

このリポジトリの文書の一覧。文書の分け方と置き場所の決め方は[ドキュメントの設計](../platform/docs/documentation.md)にある。

| 区切り | 解説 | 参照 | 手順 | ADR |
| --- | --- | --- | --- | --- |
| 全体 | [overview.md](overview.md)（狙い、考え方、構成、目指す姿） | — | — | — |
| control-plane | [app/README.md](app/README.md)（セッションのライフサイクル）、[codebase/README.md](codebase/README.md)（Go の層と書き方） | [app/usage.md](app/usage.md)（依頼の仕方）、[acp.example.yaml](../acp.example.yaml)（設定） | [app/operations.md](app/operations.md)（準備と起動） | [app/adr/](app/adr/)、[codebase/adr/](codebase/adr/) |
| target-template | [target/README.md](target/README.md)（devcontainer、CI、作業の進め方） | [target/contract.md](target/contract.md)（ターゲットの契約） | [target/README.md](target/README.md) の「使い方」 | [target/adr/](target/adr/) |
| platform | [platform/docs/README.md](../platform/docs/README.md)（プラグイン）、[documentation.md](../platform/docs/documentation.md)（ドキュメントの設計） | [harness.md](../platform/docs/harness.md)（ハーネスの契約）、[glossary.md](../platform/docs/glossary.md)（用語集） | 各プラグインのスキル | [platform/docs/adr/](../platform/docs/adr/) |

目指す姿は、今の方針として overview.md に書く。そこへの道筋は Issue にある（[作業の進め方](target/README.md#作業の進め方)）。
