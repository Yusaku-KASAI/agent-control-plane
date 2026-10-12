# conventions の全体像

## 構成

conventions は principles を前提にする。conventions を有効にするリポジトリは、principles も有効にする。

conventions は、決まりの**定義**と、それをエージェントに**届ける形**に分かれる。届ける形は定義を前提にし、
定義は届ける形を知らない。

```
conventions/
  docs/            定義：書き方、作業の進め方、文書の体系、形式、用語集（README.md が一覧）
  rules/           届け方：いつも守ることの要約。各項目に出典（定義）を書く
  hooks/、scripts/ 届け方：rules と、定義の文書の在りかを、セッションとサブエージェントの始めに読み込ませる
  skills/          届け方：必要なときに読む手順と観点（write-docs、design）
```

- 届ける形は、定義を要約したり選び出したりするだけで、新しい決まりを足さない。どの定義によるかを示す。届け方の
  種類と使い分けは[文書の体系](architecture/documentation.md#delivery "案内")にある。
- 拠り所は principles プラグインの原則とパターン。conventions はそれを前提にし、principles は conventions を知らない。
- 文書の決まりのうち機械で確かめられるものは、各リポジトリに置く `tools/check-docs.py` が確かめる。

<a id="map-conventions"></a>

## 地図

conventions の中の項目と前提の向き（[形式](reference/formats.md#map "案内")）。

| 項目 | 親 | 置き場所 | 前提にする |
| --- | --- | --- | --- |
| 定義 | conventions | | |
| 用語集 | 定義 | `reference/glossary.md` | |
| 書き方 | 定義 | `reference/writing.md` | 用語集 |
| 形式 | 定義 | `reference/formats.md` | 用語集 |
| 作業の進め方 | 定義 | `reference/workflow.md` | 用語集, 書き方 |
| 文書の体系 | 定義 | `architecture/documentation.md` | 用語集, 書き方, 形式, 作業の進め方 |
| 届け方 | conventions | | 定義 |
| ルール | 届け方 | `../rules/` | |
| スキル | 届け方 | `../skills/` | |
