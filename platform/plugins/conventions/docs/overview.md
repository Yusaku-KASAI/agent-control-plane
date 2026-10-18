# conventions の全体像

## 構成

conventions は principles を前提にする。conventions を有効にするリポジトリは、principles も有効にする。

conventions は、決まりの**定義**と、それをエージェントに**届ける形**に分かれる。届ける形は定義を前提にし、
定義は届ける形を前提にしない。

```
conventions/
  docs/            定義：文書（文書の体系、形式）、変更と課題（書き方、変更の扱い方、課題の扱い方）、レビュー、用語集、手順（guides/）
                   （README.md が一覧）
  rules/           届け方：いつも守ること（共通のルール）。定義を要約か案内で載せる
  hooks/、scripts/ 届け方：rules と、その在りかと、定義の文書の在りかを、セッションとサブエージェントの始めに読み込ませる。
                   Markdown を変えたターンの終わりに、文書の検査を回す
  skills/          届け方：docs/guides/ の共通の手順（と、ロールの外から入る目的の手順）を、必要なときに要約か案内で
                   読ませる（write-docs、design、review）
  bin/check-docs   文書の検査：形式のうち機械で確かめられるものを確かめるコマンド
  actions/         文書の検査を CI で回す GitHub Actions の複合アクション（check-docs）
  tests/           ターンの終わりの hook の試験
```

- 届け方が定義とどう関わるか（要約と案内、出典、例外）は[文書の体系](architecture/documentation.md#delivery "在りか")にある。
- 拠り所は principles プラグインの原則とパターン。conventions はそれを前提にし、principles は conventions を前提にしない。
- 文書の決まりのうち機械で確かめられるものは、このプラグインのコマンド `check-docs` が確かめる。検査の中身は、
  ターゲットの契約や共通のファイルに置かない（ターゲットの CI は複合アクションを呼ぶだけ）（[ADR 0019](adr/0019-document-check-as-plugin-command.md)）。

## 構成要素

定義、届け方、文書の検査の順に並べる。

### 用語集（`docs/reference/glossary.md`）

- **責務**：文書、変更と課題の決まりの言葉（区切り、文書の種類、定義と届け方など）を一か所で定める。ほかの言葉は、
  それを使う区切りの用語集にある。
- **当てはめた原則・パターン**：[ユビキタス言語と用語集](../../principles/docs/reference/principles.md#glossary)、
  [共有の核](../../principles/docs/reference/patterns.md#shared-kernel)。

### 文書の体系（`docs/architecture/documentation.md`）

- **責務**：このプロジェクト群の文書をどう分け、どこに何を書くか、決まりをどう定義して届けるかを解説する。
- **置くもの / 置かないもの**：置き場所と関係（どこに何を置き、何が何を前提にするか）と、読み手への届け方（決まりを、
  どの文書に、どう書いて読ませるか）と、書き換えの方針と、器の構成要素の説明。形の細部は形式に、書く手順は文書を書くに、理由は ADR に置く。
- **関係**：用語集を前提にする。原則とパターンを指す。
- **当てはめた原則・パターン**：[文書の種類](../../principles/docs/reference/patterns.md#diataxis)の解説。

### 形式（`docs/reference/formats.md`）

- **責務**：文書の根、構成要素の説明、ADR、リンク、量の上限、地図の形を決める。
- **関係**：`check-docs` がこの形を確かめる。
- **当てはめた原則・パターン**：[スキーマを先に](../../principles/docs/reference/principles.md#schema-first)：形を先に決め、文書を合わせる。

### 書き方（`docs/reference/writing.md`）

- **責務**：コード中のコメント、コミットメッセージ、PR に何を書くかを定義する。

### 変更の扱い方と課題の扱い方（`docs/reference/changes.md`、`issues.md`）

- **責務**：変更（PR）と課題（Issue）の扱い方。文書に置かない道筋と予定の置き場所を決める。
- **当てはめた原則・パターン**：[今と経緯を分ける](../../principles/docs/reference/principles.md#present-state)。

### レビューの扱い方（`docs/reference/reviews.md`）

- **責務**：レビューの種類（差分、全体）、観点、指摘の書き方と答え方、差分のレビューと全体のレビューの決まりを定義する。
  進める順は手順に置く。
- **関係**：正しさの観点は変更の扱い方の「変更とパターン」を、文書の観点は文書の体系と形式を前提にする。
- **当てはめた原則・パターン**：[別の目で見る](../../principles/docs/reference/patterns.md#four-eyes)、
  [観点を分けて読む](../../principles/docs/reference/patterns.md#perspective-based)、
  [指摘に重さを付ける](../../principles/docs/reference/patterns.md#labeled-comments)、
  [直しを任せて見直しを終える](../../principles/docs/reference/patterns.md#lgtm-with-comments)。

### 手順（[`docs/guides/`](guides/ "在りか")）

- **責務**：決まりに沿って作業する手順を定義する。目的の手順（変更を届ける、レビューする）と、その段で使う共通の手順
  （文書を書く、パターンを当てはめる、レビューを頼んで答える）。
- **置くもの / 置かないもの**：手順と、その場で要る最小の説明。決まりそのものは参照の文書に置き、手順からリンクする。
  そのプラグインの道具の使い方はスキルに置く。
- **関係**：用語集、文書、変更と課題、レビューを前提にする。目的の手順は共通のルールと役割ごとの指示が、共通の手順と
  ロールの外から入る目的の手順はスキルが、要約か案内で載せる。
- **当てはめた原則・パターン**：[文書の種類](../../principles/docs/reference/patterns.md#diataxis)の手順。

### 共通のルール（[`rules/common.md`](../rules/common.md "在りか")）

- **責務**：いつも守ることを、セッションとサブエージェントの始めに読み込ませる。
- **置くもの / 置かないもの**：定義の要約と案内だけ（届け方の決まりに沿う）。
- **当てはめた原則・パターン**：[段階的な開示](../../principles/docs/reference/principles.md#progressive-disclosure)：常に読まれるので短くする。
  [仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)：常に読み込む決まりの段。

### hook（`hooks/`、`scripts/`）

- **責務**：セッションとサブエージェントの始めに、共通のルールと、その在りか（途中で立ち返るため）と、conventions と principles の文書の在りかを読み込ませる
  （`scripts/context`）。ターンの終わりに、Markdown を変えていれば文書の検査を回し、落ちていればターンを止める
  （`scripts/check-docs-at-stop`、[ADR 0019](adr/0019-document-check-as-plugin-command.md)）。
- **置くもの / 置かないもの**：ターンの終わりの hook は、ブランチが既定のブランチから分かれてから変えた Markdown か、
  作業ツリーで変えたか足した Markdown があるときだけ回す。誰が変えたかは区別しない。既定のブランチが分からないときは
  作業ツリーだけを見る。同じ出力で止めるのは一つのセッションで一度だけで、
  止めたことを覚えられなければ止めない。何を確かめるかは置かず、`check-docs` に任せる。
- **関係**：文書の在りかは、このプラグインの場所と、同じマーケットプレイスにある principles の場所から決める。共通のルールの
  相対リンクは、読み込ませるときに絶対パスに書き換える。試験は `tests/check-docs-at-stop-test.sh`。
- **当てはめた原則・パターン**：[段階的な開示](../../principles/docs/reference/principles.md#progressive-disclosure)：中身ではなく在りかを渡す。

### 文書を書く（[`skills/write-docs/`](../skills/write-docs/SKILL.md "在りか")）

- **責務**：文書を足したり直したりするときに、[文書を書く](guides/write-docs.md "在りか")の手順を読ませる。
- **置くもの / 置かないもの**：手順への案内と、その要約だけ。手順そのものは置かない。
- **当てはめた原則・パターン**：[SSOT](../../principles/docs/reference/principles.md#ssot)。

### design スキル（[`skills/design/`](../skills/design/SKILL.md "在りか")）

- **責務**：設計するときとレビューするときに、[パターンを当てはめる](guides/design.md "在りか")の手順を読ませる。
- **置くもの / 置かないもの**：手順への案内と、その要約だけ。パターンの定義と手順そのものは置かない。
- **当てはめた原則・パターン**：[SSOT](../../principles/docs/reference/principles.md#ssot)。

### review スキル（[`skills/review/`](../skills/review/SKILL.md "在りか")）

- **責務**：自分の変更のレビューを頼んで答えるときに[レビューを頼んで答える](guides/request-review.md "在りか")の手順を、
  頼まれてレビューするときに[レビューする](guides/review.md "在りか")の手順を読ませる。
- **置くもの / 置かないもの**：手順への案内と、その要約と、github スキルへの案内（PR のレビューの残し方）。

### 文書の検査（`bin/check-docs`）

- **責務**：形式と器の決まりのうち、機械で確かめられるものを、作業ディレクトリを含むリポジトリの Markdown について確かめる。
  `--map` で、すべての地図から設計構造行列を出す。
- **置くもの / 置かないもの**：何を確かめるかは[形式](reference/formats.md "在りか")の各節の「検査」にある。文の書き方や置き場所の
  良し悪しは確かめない（レビューの観点に任せる）。
- **関係**：形式に沿う。プラグインの `bin/` なので、エージェントは `check-docs` で呼べる。人はマーケットプレイスの
  `plugins/conventions/bin/check-docs` を、確かめるリポジトリの中で実行する。ターンの終わりの hook が回す。
  CI では複合アクションが回す。リポジトリが自分の `make` から呼ぶのは、そのリポジトリの選択（[ADR 0019](adr/0019-document-check-as-plugin-command.md)）。
- **当てはめた原則・パターン**：[仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)、[Docs as Code](../../principles/docs/reference/patterns.md#docs-as-code)。

### CI の文書の検査（[`actions/check-docs/`](../actions/check-docs/action.yml "在りか")）

- **責務**：ターゲットの CI で、そのリポジトリをチェックアウトし、アクションと同じ版の `bin/check-docs` を回す。
- **置くもの / 置かないもの**：チェックアウトと `check-docs` の呼び出しだけ。何を確かめるかは置かない。
- **関係**：ターゲットの `ci.yml` が `@main` で呼ぶ（target-template の [CI](../../../../template/docs/architecture/ci.md "在りか")）。
  devcontainer のイメージを使わず、ランナーの `python3` と `git` で動く。
- **当てはめた原則・パターン**：[SSOT](../../principles/docs/reference/principles.md#ssot)：検査の具体を platform の一か所に置き、ターゲットは
  呼ぶ一行だけを書く。

<a id="map-conventions"></a>

## 地図

conventions の中の項目と前提の向き（[形式](reference/formats.md#map "在りか")）。

| 項目 | 親 | 置き場所 | 前提にする |
| --- | --- | --- | --- |
| 定義 | conventions | | |
| 用語集 | 定義 | `reference/glossary.md` | |
| 文書 | 定義 | `architecture/documentation.md`, `reference/formats.md` | 用語集 |
| 変更と課題 | 定義 | `reference/writing.md`, `reference/changes.md`, `reference/issues.md` | 用語集, 文書 |
| レビュー | 定義 | `reference/reviews.md` | 用語集, 文書, 変更と課題 |
| 手順 | 定義 | `guides/` | 用語集, 文書, 変更と課題, レビュー |
| 届け方 | conventions | | 定義 |
| 共通のルール | 届け方 | `../rules/` | |
| スキル | 届け方 | `../skills/` | |