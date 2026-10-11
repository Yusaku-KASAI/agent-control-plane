# docs

全体の狙い、考え方、構成、目指す姿は [overview.md](overview.md) にある。

ドキュメントは、届く先（誰が読めるか）で区切ってディレクトリを分ける（[platform ADR 0005](../platform/docs/adr/0005-documents-by-distribution-context.md)）。
書く前に、どの区切りの話かを決める。

| ディレクトリ | 区切り | 中身 |
| --- | --- | --- |
| `app/` | control-plane | アプリケーションそのもの（何をするか、ドメイン、システム構成）。[README](app/README.md) |
| `codebase/` | control-plane | Go アプリとしての作り方（レイヤ構成、コーディング規約、テスト） |
| `target/` | target-template | ターゲットを成り立たせる環境（devcontainer、エージェント向け設定） |
| `../platform/docs/` | platform | ターゲットをまたいで使うエージェントの能力（ロール、プラグイン）、ハーネスの契約、用語集。エージェントに届くよう、マーケットプレイスの中に置く |

区切りの中では、文書を種類（手順、参照、解説、ADR）で分け、1 つの文書には 1 つの種類だけを書く。

| 区切り | 解説 | 参照 | 手順 |
| --- | --- | --- | --- |
| control-plane | [overview.md](overview.md)、[app/README.md](app/README.md)、[codebase/README.md](codebase/README.md) | [app/usage.md](app/usage.md)、[acp.example.yaml](../acp.example.yaml) | [app/operations.md](app/operations.md) |
| target-template | [target/README.md](target/README.md) | [target/contract.md](target/contract.md) | [target/README.md](target/README.md) の「使い方」 |
| platform | [platform/docs/README.md](../platform/docs/README.md) | [harness.md](../platform/docs/harness.md)、[glossary.md](../platform/docs/glossary.md) | 各プラグインのスキル |

## ADR

- 置き場所は各ディレクトリの `adr/NNNN-<slug>.md`。番号はディレクトリごとの連番。
- 見出しは Status / Date / Context / Decision / Alternatives / Consequences。
- ADR はその時点の判断の記録なので、後から内容を書き換えない。判断を変えるときは新しい ADR を作り、
  古い方の Status に、全体を覆すなら `Superseded by NNNN`、一部を変えるなら `Amended by NNNN` を付ける。

## ドキュメントと Issue の分担

- ドキュメント：今の姿と、今の方針。目指す姿も、今の方針として [overview.md](overview.md) に書く。
- ADR：その方針を選んだ理由と、選ばなかった案。
- Issue：目指す姿への道筋。何をどの順でやるか、どこまで進んだか（[作業の進め方](target/README.md#作業の進め方)）。
