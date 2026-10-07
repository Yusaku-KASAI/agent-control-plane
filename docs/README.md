# docs

全体の狙い、考え方、構成、目指す姿は [overview.md](overview.md) にある。

ドキュメントは関心ごとにディレクトリを分ける。書く前に、どの関心の話かを決める。

| ディレクトリ | 関心 |
| --- | --- |
| `app/` | control-plane というアプリケーションそのもの（何をするか、ドメイン、システム構成）。[README](app/README.md) |
| `codebase/` | Go アプリとしての作り方（レイヤ構成、コーディング規約、テスト） |
| `target/` | このリポジトリを「エージェントが作業するターゲット」として成り立たせる環境（devcontainer、エージェント向け設定）。target-template の抽出元 |
| `platform/` | ターゲットをまたいで使うエージェントの能力（ロール、プラグイン）。`platform/` ディレクトリの設計で、agent-platform の抽出元 |

## ADR

- 置き場所は `<関心>/adr/NNNN-<slug>.md`。番号は関心ごとの連番。
- 見出しは Status / Date / Context / Decision / Alternatives / Consequences。
- ADR はその時点の判断の記録なので、後から内容を書き換えない。判断を変えるときは新しい ADR を作り、
  古い方の Status に、全体を覆すなら `Superseded by NNNN`、一部を変えるなら `Amended by NNNN` を付ける。

## ドキュメントと Issue の分担

- ドキュメント：今の姿と、今の方針。目指す姿も、今の方針として [overview.md](overview.md) に書く。
- ADR：その方針を選んだ理由と、選ばなかった案。
- Issue：目指す姿への道筋。何をどの順でやるか、どこまで進んだか（[作業の進め方](target/README.md#作業の進め方)）。
