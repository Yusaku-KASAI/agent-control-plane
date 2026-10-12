# conventions の全体像

## 構成

conventions は principles を前提にする。conventions を有効にするリポジトリは、principles も有効にする。

conventions は、決まりの**定義**と、それをエージェントに**届ける形**に分かれる。届ける形は定義を前提にし、
定義は届ける形を前提にしない。

```
conventions/
  docs/            定義：文書（文書の体系、形式）、変更と課題（書き方、変更の扱い方、課題の扱い方）、用語集
                   （README.md が一覧）
  rules/           届け方：いつも守ることの要約。各項目に出典（定義）を書く
  hooks/、scripts/ 届け方：rules と、定義の文書の在りかを、セッションとサブエージェントの始めに読み込ませる
  skills/          届け方：必要なときに読む手順と観点（write-docs、design）
```

- 届け方が定義とどう関わるか（写しであること、出典、例外）は[文書の体系](architecture/documentation.md#delivery "案内")にある。
- 拠り所は principles プラグインの原則とパターン。conventions はそれを前提にし、principles は conventions を前提にしない。
- 文書の決まりのうち機械で確かめられるものは、各リポジトリに置く `tools/check-docs.py` が確かめる。

## 構成要素

### 書き方（`docs/reference/writing.md`）

- **責務**：コード中のコメント、コミットメッセージ、PR に何を書くかを定義する。

### 形式（`docs/reference/formats.md`）

- **責務**：構成要素の説明、ADR、リンクの形を決める。
- **関係**：check-docs.py がこの形を確かめる。
- **当てはめた原則・パターン**：[スキーマを先に](../../principles/docs/reference/principles.md#schema-first)：形を先に決め、文書を合わせる。

### 文書を書く（`skills/write-docs/`）

- **責務**：文書を足したり直したりするときの手順（区切り → 種類 → 持ち主 → 形 → 確かめる）。
- **当てはめた原則・パターン**：[文書の種類](../../principles/docs/reference/patterns.md#diataxis)の手順。

### 用語集（`docs/reference/glossary.md`）

- **責務**：文書、変更と課題の決まりの言葉（区切り、文書の種類、構成要素の説明など）を一か所で定める。ほかの言葉は、
  それを使う区切りの用語集にある。
- **当てはめた原則・パターン**：[ユビキタス言語と用語集](../../principles/docs/reference/principles.md#glossary)、
  [共有の核](../../principles/docs/reference/patterns.md#shared-kernel)。

### 変更の扱い方と課題の扱い方（`docs/reference/changes.md`、`issues.md`）

- **責務**：変更（PR）と課題（Issue）の扱い方。文書に置かない道筋と予定の置き場所を決める。
- **当てはめた原則・パターン**：[今と経緯を分ける](../../principles/docs/reference/principles.md#present-state)。

### 共通のルール（`rules/`）

- **責務**：いつも守ることを、要約の形でセッションとサブエージェントの始めに読み込ませる（届け方）。
- **置くもの / 置かないもの**：定義の要約だけ（届け方の決まりに沿う）。
- **当てはめた原則・パターン**：[段階的な開示](../../principles/docs/reference/principles.md#progressive-disclosure)：常に読まれるので短くする。
  [仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)：常に読み込む決まりの段。

### hook（`hooks/`、`scripts/context`）

- **責務**：セッションとサブエージェントの始めに、共通のルールと、conventions と principles の文書の在りかを読み込ませる。
- **関係**：文書の在りかは、このプラグインの場所と、同じマーケットプレイスにある principles の場所から決める。
- **当てはめた原則・パターン**：[段階的な開示](../../principles/docs/reference/principles.md#progressive-disclosure)：中身ではなく在りかを渡す。

### design スキル（`skills/design/`）

- **責務**：どの変更にどのパターンを当てはめるかと、レビューでの指摘の仕方。
- **置くもの / 置かないもの**：パターンの定義は置かず、パターンの一覧を指す。
- **当てはめた原則・パターン**：[SSOT](../../principles/docs/reference/principles.md#ssot)。

### 文書の検査（`tools/check-docs.py`）

- **責務**：形式と器の決まりのうち、機械で確かめられるものを `make check-docs` で確かめる。
- **置くもの / 置かないもの**：何を確かめるかは[形式](reference/formats.md "案内")の各節の「検査」にある。文の書き方や置き場所の
  良し悪しは確かめない（レビューの観点に任せる）。
- **関係**：形式に沿う。どのリポジトリにも同じものを置く。
- **当てはめた原則・パターン**：[仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)、[Docs as Code](../../principles/docs/reference/patterns.md#docs-as-code)。

<a id="map-conventions"></a>

## 地図

conventions の中の項目と前提の向き（[形式](reference/formats.md#map "案内")）。

| 項目 | 親 | 置き場所 | 前提にする |
| --- | --- | --- | --- |
| 定義 | conventions | | |
| 用語集 | 定義 | `reference/glossary.md` | |
| 文書 | 定義 | `architecture/documentation.md`, `reference/formats.md` | 用語集 |
| 変更と課題 | 定義 | `reference/writing.md`, `reference/changes.md`, `reference/issues.md` | 用語集, 文書 |
| 届け方 | conventions | | 定義 |
| ルール | 届け方 | `../rules/` | |
| スキル | 届け方 | `../skills/` | |