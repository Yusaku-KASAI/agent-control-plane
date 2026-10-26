# 変更の扱い方

どのリポジトリでも同じにする、変更（PR）の扱い方。変更を進める順は手順の[変更を届ける](../guides/deliver-change.md "在りか")に、課題の扱い方は
[課題](issues.md "在りか")にある。

<a id="changes"></a>

## 変更

- 既定のブランチへの変更は PR 経由で行い、merge commit でマージする。PR のコミットがそのまま残る。
- PR は既定のブランチから切る。まだマージされていない PR の中身を前提にする変更（同じ所を変える、その PR で
  足したものを使う）だけは、その PR のブランチから切って重ね、向き先をそのブランチにする。
  - 本文に書くことは[書き方：PR](writing.md#pull-requests)にある。マージは下から順にする。
  - 下の PR に push したら、上の PR のブランチに取り込む（merge）。
  - 下の PR がマージされたら、上の PR の向き先が既定のブランチに変わったことを確かめ、本文から重ねたことの記述を
    消す。
  - 下の PR がマージされずに閉じたら、上の PR の向き先を既定のブランチに変えない（採らなかった下の中身が入る）。
    上の扱いを依頼者に確かめる。
- PR とコミットに何を書くかは[書き方](writing.md "在りか")にある。

<a id="patterns"></a>

## 変更とパターン

| 当てはまる変更 | 当てはめるパターン |
| --- | --- |
| どの変更も | [一つの目的の小さな変更](../../../principles/docs/reference/patterns.md#small-change) |
| 状態によって振る舞いが変わる | [状態を軸で洗い出す](../../../principles/docs/reference/patterns.md#state-axes)、[事実と判定を分ける](../../../principles/docs/reference/patterns.md#facts-and-judgement) |
| 消す、上書きする、作り直す | [失われうるものを先に挙げる](../../../principles/docs/reference/patterns.md#loss-first) |
| 目的の手順を足す、ロールの受け持ちを変える | [一つの出典から届ける](../../../principles/docs/reference/patterns.md#single-source)（並べた所の一覧は[目的の手順と共通の手順](../architecture/documentation.md#procedures "在りか")） |
| 検査や試験を足す、変える | [壊した版で検査が落ちることを確かめる](../../../principles/docs/reference/patterns.md#mutation-testing)（何を検査にし、どこに置くかは[検査にするものと置き場所](../architecture/documentation.md#checks "在りか")） |
| テストダブル（試験で本物の代わりに使うもの）を足す、変える | [契約の試験](../../../principles/docs/reference/patterns.md#contract-test) |
| 人が実行する移行や運用の手順を含む | [手順を実装と突き合わせる](../../../principles/docs/reference/patterns.md#verify-procedures) |
| 後から理由を知りたくなるか、間違えそうな判断を含む（例：構成、決まりの方針、契約、形式、データの形を選ぶか変える。言い回しや置き場所の直しは含まない） | [判断の記録](../../../principles/docs/reference/patterns.md#adr)（置き場所と形は[形式](formats.md#adr "在りか")） |
| 決まり（定義）を足す、変える、消す | [一つの出典から届ける](../../../principles/docs/reference/patterns.md#single-source)（直すものは[定義と届け方](../architecture/documentation.md#delivery "在りか")） |

<a id="checks"></a>

## 検査と履歴

- コミットの前に `git status` で、入れるファイルを確かめてから足す。`git add -A` は、意図しないファイル（試しに
  動かしたコマンドが作ったものなど）まで入れる。
- push の前に、手元で CI と同じ検査を通す。どのコマンドかはそのリポジトリの決まりにある（ターゲットなら
  [ターゲットの契約の検査](../../../../docs/reference/target.md#checks "在りか")）。
  CI は手元で通したことの確かめで、CI で初めて失敗に気づくことがないようにする。Markdown を変えたら、文書の検査
  （conventions のコマンド `check-docs`）も通す。
- push のあと、CI の結果を待って作業を止めない。結果が出たら確かめ、報告には CI の状態（通った、失敗した、実行中）を
  書く。
- CI が失敗したら、ログで原因を確かめてから直す。
- 履歴を書き換えて force push したら、PR の本文を今の中身に合わせ、何をなぜ書き換えたかをコメントに書く。
