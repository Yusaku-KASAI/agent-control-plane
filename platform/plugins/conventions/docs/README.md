# conventions

このプロジェクト群のリポジトリが共通で従う決まり：文書をどう分けて書くか、変更と課題をどう扱うか、設計で何を
当てはめるか。拠り所は principles プラグインの原則とパターンで、それ以外は前提にしない。

| 文書 | 種類 | 何があるか |
| --- | --- | --- |
| [overview.md](overview.md) | 解説 | このプラグインの構成（定義と届け方）、構成要素、地図 |
| [architecture/documentation.md](architecture/documentation.md) | 解説 | 文書の体系：置き場所と関係（設計を記述する 3 層、区切り、項目の階層と地図、文書の根と器、種類）と、読み手への届け方（定義と届け方、検査にするものと置き場所、要約と案内、起点、読む範囲、目的の手順と共通の手順）、書き換えの方針 |
| [reference/formats.md](reference/formats.md) | 参照 | 文書の形式：文書の根、構成要素の説明、入口からの道、手順、役割ごとの指示、スキル、ADR、リンク、量の上限、地図 |
| [reference/writing.md](reference/writing.md) | 参照 | 書き方：コメント、コミット、PR |
| [reference/changes.md](reference/changes.md) | 参照 | 変更の扱い方：PR の流れ、変更とパターン、検査と履歴 |
| [reference/reviews.md](reference/reviews.md) | 参照 | レビューの扱い方：種類（差分、全体）、観点、指摘の重さと書き方、結果の形、差分のレビューと全体のレビューの決まり |
| [reference/issues.md](reference/issues.md) | 参照 | 課題の扱い方：Issue の分類、ロードマップ、整っている状態、キャンバス、セッションを締めるときに持ち越すもの |
| [reference/glossary.md](reference/glossary.md) | 参照 | 文書、変更と課題の決まりで使う言葉 |
| [guides/deliver-change.md](guides/deliver-change.md) | 手順 | 変更を届ける（目的の手順）：始めに読む、依頼、今の姿、形と壊れうる場面、作る、レビュー、確かめていないこと |
| [guides/write-docs.md](guides/write-docs.md) | 手順 | 文書を書く：区切り、種類、持ち主、形、触れ方とリンク、地図、量、確かめる |
| [guides/design.md](guides/design.md) | 手順 | パターンを当てはめる：変更の種類からパターンを選び、設計とレビューに使う |
| [guides/review.md](guides/review.md) | 手順 | レビューする（目的の手順）：始めに読む、種類、差分のレビュー、全体のレビュー |
| [guides/request-review.md](guides/request-review.md) | 手順 | レビューを頼んで答える（共通の手順）：頼む、答える、終える |
| [guides/close-session.md](guides/close-session.md) | 手順 | セッションを締める（目的の手順）：始めに読む、片付けを止めているもの、洗い出す、残す、課題の今の姿、片付けられる状態にする、締めのまとめ |
| [guides/consult.md](guides/consult.md) | 手順 | 相談に乗る（目的の手順）：始めに読む、問い、調べる、広げて絞る、キャンバスに残す、答える |
| [guides/organize-issues.md](guides/organize-issues.md) | 手順 | 課題を整える（目的の手順）：始めに読む、範囲、今の姿、ずれ、整える、まとめ |
| [adr/](adr/) | 判断 | 文書、変更と課題の決まりの判断 |

エージェントには、目的の手順は起点（共通のルールと役割ごとの指示。どの手順をどこに直に並べるかは[文書の体系](architecture/documentation.md#procedures)）が、共通の手順とロールの外から入る目的の手順はスキルが届ける：文書を書く（[skills/write-docs](../skills/write-docs/SKILL.md)）、設計のパターン（[skills/design](../skills/design/SKILL.md)）、レビュー（[skills/review](../skills/review/SKILL.md)）、セッションを締める（[skills/close-session](../skills/close-session/SKILL.md)）、相談に乗る（[skills/consult](../skills/consult/SKILL.md)）、課題を整える（[skills/organize-issues](../skills/organize-issues/SKILL.md)）。
