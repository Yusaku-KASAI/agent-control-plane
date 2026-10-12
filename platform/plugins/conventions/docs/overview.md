# conventions の全体像

## 構成

conventions は principles を前提にする。conventions を有効にするリポジトリは、principles も有効にする。

```
conventions/
  rules/          常に読み込む決まり（コメント、コミット、PR と、文書の決まりの要約）
  hooks/、scripts/ セッションとサブエージェントの始めに、rules と文書の在りかを読み込ませる
  skills/         write-docs（文書を書く手順）、design（どの変更にどのパターンを当てはめるか）
  docs/           決まりの文書（README.md が一覧）
```

- 拠り所は principles プラグインの原則とパターン。conventions はそれを前提にし、principles は conventions を知らない。
- エージェントには、いつも守らせたいことだけを hook で読み込ませ、手順と詳細はスキルと文書で必要なときに読ませる。
- 文書の決まりのうち機械で確かめられるものは、各リポジトリに置く `tools/check-docs.py` が確かめる。

<a id="map-conventions"></a>

## 地図

conventions の中の項目と前提の向き（[形式](reference/formats.md#map "案内")）。

| 項目 | 親 | 置き場所 | 前提にする |
| --- | --- | --- | --- |
| 用語集 | conventions | `reference/glossary.md` | |
| 形式 | conventions | `reference/formats.md` | 用語集 |
| 作業の進め方 | conventions | `reference/workflow.md` | 用語集 |
| 文書の体系 | conventions | `architecture/documentation.md` | 用語集, 形式, 作業の進め方 |
| ルールとスキル | conventions | `../rules/`, `../skills/` | 文書の体系, 形式, 作業の進め方, 用語集 |

