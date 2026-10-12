# platform の文書

platform（エージェントの能力を配るマーケットプレイス）自身の文書の一覧。マーケットプレイスと一緒にすべての
エージェントに届く。どこでも通用する原則とパターンは [principles](../plugins/principles/docs/README.md)、
このプロジェクト群の決まり（文書の体系、作業の進め方）は [conventions](../plugins/conventions/docs/README.md) の
プラグインにある。

| 文書 | 種類 | 何があるか |
| --- | --- | --- |
| [overview.md](overview.md) | 解説 | platform の全体の構成と届き方 |
| [architecture/plugins.md](architecture/plugins.md) | 解説 | 各プラグインの責務と、プラグインの間の関係 |
| [guides/use-interactively.md](guides/use-interactively.md) | 手順 | 対話で使う準備 |
| [reference/harness.md](reference/harness.md) | 参照 | ハーネスの契約：エージェントが動く環境と渡されるもの |
| [reference/glossary.md](reference/glossary.md) | 参照 | エージェントとハーネスの言葉 |
| [adr/](adr/) | 判断 | プラグインの判断 |

各プラグインの中の構成は、それぞれの文書にある。

| プラグイン | 何があるか |
| --- | --- |
| [harness](../plugins/harness/docs/README.md) | 動き方と契約の在りかを伝える hook、切り分けのスキル |
| [roles](../plugins/roles/docs/README.md) | ロール（developer、guide）の定義 |
| [github](../plugins/github/docs/README.md) | `prctl`、PR の本文が古いことを見つける hook、GitHub での作業のスキル |
| [slack](../plugins/slack/docs/README.md) | `slackctl` と認証情報の選び方、Slack を読み書きするスキル |
| [bell](../plugins/bell/docs/README.md) | どの場面で何回ベルを鳴らすか |
