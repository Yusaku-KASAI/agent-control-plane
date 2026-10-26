# conventions の全体像

## 構成

conventions は principles を前提にする。conventions を有効にするリポジトリは、principles も有効にする。

conventions は、決まりの**定義**と、それをエージェントに届ける**届け方**に分かれる。届け方は定義を前提にし、
定義は届け方を前提にしない。定義と届け方の仕組みは[文書の体系](architecture/documentation.md#delivery "在りか")にある。

```
conventions/
  docs/            定義：用語集、決まり（検査、文書、変更と課題、レビュー）、手順（guides/）と、全体の解説（文書の体系）。
                   一覧は docs/README.md
  rules/           届け方：いつも守ること（共通のルール）。定義を要約か案内で載せる
  hooks/、scripts/ 届け方：共通のルールを始めに読み込ませる hook と、Markdown を変えたターンの終わりに文書の検査を回す hook
  skills/          届け方：docs/guides/ の手順への入口
  bin/check-docs   文書の検査：構造の決まりのうち検査にしたものを確かめるコマンド。地図と文書の一覧も出す
  actions/         文書の検査を CI で回す GitHub Actions の複合アクション（check-docs）
  tests/           hook と文書の一覧の試験
```

- 拠り所は principles プラグインの原則とパターン。conventions はそれを前提にし、principles は conventions を前提にしない。
- 文書の構造の決まりのうち検査にしたもの（形式の各節の「検査」）は、このプラグインのコマンド `check-docs` が確かめる。何をどう確かめるかは、
  ターゲットの契約や共通のファイルに置かない（ターゲットの CI は複合アクションを呼ぶだけ）（[ADR 0019](adr/0019-document-check-as-plugin-command.md)）。

## 構成要素

定義、届け方、文書の検査の順に並べる。定義の文書の一覧は[索引](README.md "在りか")にある。

### 用語集（`docs/reference/glossary.md`）

- **責務**：文書と作業の決まりの言葉（区切り、文書の種類、定義と届け方など）の意味を一か所で定める。ほかの言葉は、
  それを使う区切りの用語集にある。
- **当てはめた原則・パターン**：[ユビキタス言語と用語集](../../principles/docs/reference/principles.md#glossary)、
  [共有の核](../../principles/docs/reference/patterns.md#shared-kernel)。

### 文書の体系（`docs/architecture/documentation.md`）

- **責務**：このプロジェクト群で、文書をどう置き、決まりをどう定義して届けるかの仕組みを解説する。
- **置くもの / 置かないもの**：仕組み（3 つの層、区切りと依存の向き、項目の階層と地図、文書の根と種類、定義と届け方、
  手順と役割）と、届け方の構成要素の説明。どの作業でも始めに全体まで読まれるので、決まりの細部は参照の文書に置く。
- **関係**：地図ではどの項目にも入らない conventions の全体で、conventions の中のどの文書からも前提にされ、それらを
  在りかで指す。原則とパターンを前提にする。
- **当てはめた原則・パターン**：[文書の種類で分ける](../../principles/docs/reference/patterns.md#diataxis)の解説。
  [段階的な開示](../../principles/docs/reference/principles.md#progressive-disclosure)。

### 文書の決まり（`docs/reference/formats.md`、`delivery.md`）

- **責務**：形式は、文書の検査の範囲と、文書の根と器、構成要素の説明、入口からの道、手順、役割ごとの指示、スキル、ADR、
  リンク、量の上限、地図の形を決める。届け方の決まりは、要約と案内、起点に何をどう載せるか、目的の手順と共通の手順を決める。
- **関係**：検査の扱い方を前提にする。`check-docs` が形式の形を確かめる。
- **当てはめた原則・パターン**：[スキーマを先に](../../principles/docs/reference/principles.md#schema-first)：形を先に決め、文書を合わせる。
  [一つの出典から届ける](../../principles/docs/reference/patterns.md#single-source)。

### 検査の扱い方（`docs/reference/checks.md`）

- **責務**：何を検査にし、検査をどこに置くかを、文書にもコードにも当てはまる形で決める。
- **関係**：区切りと依存の向き（文書の体系）を前提にする。
- **当てはめた原則・パターン**：[仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)。

### 変更と課題の決まり（`docs/reference/writing.md`、`changes.md`、`issues.md`）

- **責務**：書き方は、理由の置き場所と、コメント、コミットメッセージ、PR に何を書くかを決める。変更の扱い方と課題の
  扱い方は、変更（PR）と課題（Issue）の扱い方と、文書に置かない道筋と予定の置き場所を決める。
- **当てはめた原則・パターン**：[今と経緯を分ける](../../principles/docs/reference/principles.md#present-state)。

### レビューの扱い方（`docs/reference/reviews.md`）

- **責務**：レビューの種類（差分、全体）、観点、指摘の書き方と答え方、回し方（通常と念入り）、差分のレビューと全体の
  レビューの決まりを定義する。進める順は手順に置く。
- **関係**：正しさの観点は変更の扱い方の「変更とパターン」を、文書の観点は文書の決まりを、検査の観点は検査の扱い方を前提にする。
- **当てはめた原則・パターン**：[別の目で見る](../../principles/docs/reference/patterns.md#four-eyes)、
  [観点を分けて読む](../../principles/docs/reference/patterns.md#perspective-based)、
  [指摘に重さを付ける](../../principles/docs/reference/patterns.md#labeled-comments)、
  [直しを任せて見直しを終える](../../principles/docs/reference/patterns.md#lgtm-with-comments)。

### 手順（[`docs/guides/`](guides/ "在りか")）

- **責務**：決まりに沿って作業する手順を定義する。目的の手順と、その段で使う共通の手順に分かれる（一覧は[索引](README.md "在りか")）。
- **置くもの / 置かないもの**：手順と、その場で要る最小の説明。決まりそのものは参照の文書に置き、手順からリンクする。
- **関係**：用語集と決まりを前提にする。目的の手順は共通のルールと役割ごとの指示が、手順への入口はスキルが届ける。
- **当てはめた原則・パターン**：[文書の種類で分ける](../../principles/docs/reference/patterns.md#diataxis)の手順。

### 共通のルール（[`rules/common.md`](../rules/common.md "在りか")）

- **責務**：いつも守ることを、セッションとサブエージェントの始めに読み込ませる。
- **置くもの / 置かないもの**：定義の要約と案内だけ（[起点](reference/delivery.md#starting-points "在りか")の決まりに沿う）。
- **当てはめた原則・パターン**：[段階的な開示](../../principles/docs/reference/principles.md#progressive-disclosure)：常に読まれるので短くする。
  [仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)：常に読み込む決まりの段。

### 読み込ませる hook（`hooks/`、[`scripts/context`](../scripts/context "在りか")）

- **責務**：セッションとサブエージェントの始めに、共通のルールと、その在りか（途中で立ち返るため）と、conventions と
  principles の文書の在りかを読み込ませる。
- **関係**：文書の在りかは、このプラグインの場所と、同じマーケットプレイスにある principles の場所から決める。共通の
  ルールの相対リンクは、読み込ませるときに絶対パスに書き換える。
- **当てはめた原則・パターン**：[段階的な開示](../../principles/docs/reference/principles.md#progressive-disclosure)：中身ではなく在りかを渡す。

### ターンの終わりの文書の検査（`hooks/`、[`scripts/check-docs-at-stop`](../scripts/check-docs-at-stop "在りか")）

- **責務**：ターンの終わりに、Markdown を変えていれば文書の検査を回し、落ちていればターンを止める
  （[ADR 0019](adr/0019-document-check-as-plugin-command.md)）。
- **置くもの / 置かないもの**：ブランチが既定のブランチから分かれてから変えた Markdown か、作業ツリーで変えたか足した
  Markdown があるときだけ回す。誰が変えたかは区別しない。既定のブランチが分からないときは作業ツリーだけを見る。同じ
  出力で止めるのは一つのセッションで一度だけで、止めたことを覚えられなければ止めない。何を確かめるかは置かず、
  `check-docs` に任せる。
- **関係**：試験は `tests/check-docs-at-stop-test.sh`。
- **当てはめた原則・パターン**：[仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)。

### スキル（[`skills/`](../skills/ "在りか")）

- **責務**：使うと決めたときに、手順を全体まで読ませ、破られやすいことを要約で示す。
- **置くもの / 置かないもの**：手順への案内と、その要約だけ。手順そのものと決まりは置かない。

  | スキル | 届ける手順 |
  | --- | --- |
  | [write-docs](../skills/write-docs/SKILL.md "在りか") | 文書を書く |
  | [review](../skills/review/SKILL.md "在りか") | レビューを頼んで答える、レビューする |
  | [close-session](../skills/close-session/SKILL.md "在りか") | セッションを締める |
  | [consult](../skills/consult/SKILL.md "在りか") | 相談に乗る |
  | [organize-issues](../skills/organize-issues/SKILL.md "在りか") | 課題を整える |

- **当てはめた原則・パターン**：[SSOT](../../principles/docs/reference/principles.md#ssot)。

### 文書の検査（`bin/check-docs`）

- **責務**：文書の構造の決まりのうち、検査にしたもの（形式の各節の「検査」に挙げたもの）を、作業ディレクトリを含む
  リポジトリの Markdown について確かめる。`--map` で、すべての地図から設計構造行列を出す。`--catalog` で、文書ごとの
  属性（区分、タイトル、版と更新日（作業ツリーで書き換えていれば版に `+`）、ADR の状態、地図の項目、前提にする文書と
  在りかを示すだけの文書）を、一次の情報（置き場所、見出し、git の履歴、ADR の frontmatter、地図、リンク）から導いて、
  タブ区切りの一覧に出す。前提をリンクでなくパスで書きうる文書（役割ごとの指示とスキル）の行は、リンクから導けない
  前提がありうることを示すため、前提と在りかの末尾に `?` を付ける。属性は文書に手で書かず、導く
  （[ADR 0026](adr/0026-derive-document-catalog.md)）。
- **置くもの / 置かないもの**：何を確かめるかは[形式](reference/formats.md "在りか")の各節の「検査」にある。文の書き方や、文書が
  述べること（中身。[形式：検査の範囲](reference/formats.md#check-scope "在りか")）は確かめない（レビューの観点に任せる）。
- **関係**：形式に沿う。プラグインの `bin/` なので、エージェントは `check-docs` で呼べる。人はマーケットプレイスの
  `plugins/conventions/bin/check-docs` を、確かめるリポジトリの中で実行する。ターンの終わりの hook が回す。一覧の試験は
  `tests/check-docs-catalog-test.sh`。CI では複合アクションが回す。リポジトリが自分の `make` から呼ぶのは、そのリポジトリの
  選択（[ADR 0019](adr/0019-document-check-as-plugin-command.md)）。
- **当てはめた原則・パターン**：[仕組みで守る](../../principles/docs/reference/principles.md#mechanisms)、[Docs as Code](../../principles/docs/reference/patterns.md#docs-as-code)。

### CI の文書の検査（[`actions/check-docs/`](../actions/check-docs/action.yml "在りか")）

- **責務**：CI で、そのリポジトリをチェックアウトし、アクションと同じ版の `bin/check-docs` を回す。
- **置くもの / 置かないもの**：チェックアウトと `check-docs` の呼び出しだけ。何を確かめるかは置かない。
- **関係**：使うリポジトリの CI のワークフローから呼ばれる。devcontainer のイメージを使わず、ランナーの `python3` と
  `git` で動く。
- **当てはめた原則・パターン**：[SSOT](../../principles/docs/reference/principles.md#ssot)：検査の具体を conventions の一か所に置き、
  使うリポジトリは呼ぶ一行だけを書く。

<a id="map-conventions"></a>

## 地図

conventions の中の項目と前提の向き（[形式](reference/formats.md#map "在りか")）。

| 項目 | 親 | 置き場所 | 前提にする |
| --- | --- | --- | --- |
| 定義 | conventions | | |
| 用語集 | 定義 | `reference/glossary.md` | |
| 決まり | 定義 | | 用語集 |
| 検査 | 決まり | `reference/checks.md` | |
| 文書 | 決まり | `reference/formats.md`, `reference/delivery.md` | 検査 |
| 変更と課題 | 決まり | `reference/writing.md`, `reference/changes.md`, `reference/issues.md` | 文書 |
| レビュー | 決まり | `reference/reviews.md` | 文書, 検査, 変更と課題 |
| 手順 | 定義 | `guides/` | 用語集, 決まり |
| 届け方 | conventions | | 定義 |
| 共通のルール | 届け方 | `../rules/` | |
| スキル | 届け方 | `../skills/` | |
