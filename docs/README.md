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

## ADR

各ディレクトリの `adr/` に置く。形（frontmatter、見出し、変えるときの扱い）は[ターゲットの契約](target/contract.md#adr)にあり、
`make check-docs` で確かめる。

## ドキュメントと Issue の分担

- ドキュメント：今の姿と、今の方針。目指す姿も、今の方針として [overview.md](overview.md) に書く。
- ADR：その方針を選んだ理由と、選ばなかった案。
- Issue：目指す姿への道筋。何をどの順でやるか、どこまで進んだか（[作業の進め方](target/README.md#作業の進め方)）。
