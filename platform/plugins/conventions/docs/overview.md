# conventions の全体像

## 構成

```
conventions/
  rules/          常に読み込む決まり（コメント、コミット、PR と、文書の決まりの要約）
  hooks/、scripts/ セッションとサブエージェントの始めに、rules と文書の在りかを読み込ませる
  skills/         write-docs（文書を書く手順）、design（どの変更にどのパターンを当てはめるか）
  docs/           決まりの文書（README.md が一覧）
```

- 拠り所は principles プラグインの原則とパターン。conventions はそれを前提にし、principles は conventions を知らない。
- エージェントには、いつも守らせたいことだけを hook で読み込ませ、手順と詳細はスキルと文書で必要なときに読ませる。
- 文書の決まりのうち機械で確かめられるものは、各リポジトリの `tools/check-docs.py`（target-template の共通のファイル）が確かめる。
