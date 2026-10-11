# 文書の体系

どのリポジトリでも、文書をどう分け、どこに何を書くか。判断の理由は
[ADR 0005](../adr/0005-documents-by-distribution-context.md) と [ADR 0007](../adr/0007-type-first-docs-and-platform-kernel.md) にある。
原則は[設計の原則](../reference/principles.md)、パターンは[設計のパターン](../reference/patterns.md)、形は[文書の形式](../reference/formats.md)、書くときの手順は
[文書を書く](../guides/write-docs.md)にある。

## 構成

### 設計を記述する 3 つの層

| 層 | 書くもの | 置き場所 |
| --- | --- | --- |
| 原則とパターン | 原則（何を大事にするか）と、それを特定の問題への形にしたパターン（どう組むか）。一般的な当てはめ方とよくある失敗まで書き、どこで使うかは書かない | platform の [reference/principles.md](../reference/principles.md) と [reference/patterns.md](../reference/patterns.md) |
| アーキテクチャ | そのリポジトリで採った全体の構成と、部分の間の関係 | 各リポジトリの `docs/overview.md` と、`docs/architecture/` の各文書の冒頭 |
| 構成要素の説明 | 層、ディレクトリ、ファイルごとの責務と関係、当てはめた原則・パターン | `docs/architecture/` の各文書。[形式](../reference/formats.md#component)に沿う |

構成要素の説明から原則やパターンへリンクし、それがそこでどう効いているかを書く。一つの原則やパターンは多くの
構成要素に、一つの構成要素は多くの原則やパターンに当てはまる。

### 区切りと依存の向き

知識は、使う人（誰が読めるか）と持ち主が同じものを一つの区切りにまとめ、区切りごとにリポジトリを分ける。
切り出すまでは、このリポジトリに 3 つの区切りが同居する。

| 区切り | 置くもの | 届く先 | 今の場所 |
| --- | --- | --- | --- |
| platform | 原則とパターン、文書の体系と形式、作業の進め方、ハーネスの契約、用語集、ロール、スキル | すべてのエージェント（マーケットプレイスと一緒に `docs/` が届く） | `platform/` |
| target-template | ターゲットの契約、devcontainer、CI | すべてのターゲット（テンプレートから作る） | `template/docs/` と、ルートの環境のファイル |
| control-plane | 全体の設計、アプリの仕組み、Go の書き方、運用の手順 | control-plane の開発者 | `docs/` |
| 各ターゲット | プロダクトの前提、そのリポジトリの仕組みと判断 | そのターゲットのエージェント | 各リポジトリ |

- platform は、すべての区切りに届く[共有の核](../reference/patterns.md#shared-kernel)。ほかの区切りは platform の文書を前提にしてよい。
  platform はほかの区切りの文書を前提にしない（[依存の向き](../reference/principles.md#dependency-rule)）。
- platform 以外の区切りの間をつなぐのは、契約だけ。ターゲットの契約（target-template にある）と、ハーネスの契約
  （使うエージェントに届くよう platform にある）。どちらも[使う側に届く契約](../reference/patterns.md#published-language)で、
  実装との一致は[契約の試験](../reference/patterns.md#contract-test)で確かめる。契約は使う側から読める場所に置き、作る側はそれに合わせて作る。

### 各リポジトリの器

各リポジトリは、次の構成で文書を持つ。

```
README.md
CLAUDE.md
docs/
  README.md
  overview.md
  architecture/
  guides/
  reference/
  adr/
```

一つのリポジトリに関心の違う部分があるときは、`architecture/` と `adr/` を `<関心>/` に分ける（例：control-plane の
`app/` と `codebase/`）。`guides/` と `reference/` は、文書が増えたら同じように分ける。

### 文書の外

コード中のコメント、コミットメッセージ、PR に何を書くかは共通のルール（rules プラグイン）にある。道筋と予定は
Issue に置き、文書には書かない（[作業の進め方](../reference/workflow.md)）。チュートリアルは置かない。使う人が限られ、
`guides/` の手順で足りる。

### 体系を作る部品

体系は、各リポジトリが持つ上の器と、それを決めて守らせる platform の部品からなる。

```
原則（principles.md）◀──拠り所──パターン（patterns.md）
   ▲                                   ▲
   └──── 構成要素の説明がリンクする ────┘
文書の体系（この文書）──形──▶ 形式（formats.md）◀──確かめる── check-docs.py
   │                         ▲
   └──手順──▶ 文書を書く（write-docs.md）
共通のルール（rules）＝ 常に読み込む要約 ──指す──▶ 原則、この文書
design スキル（review）──どの変更にどのパターンか──▶ パターン
用語集（glossary.md）：体系の言葉も含め、区切りをまたぐ言葉
```

## 構成要素

各リポジトリの器と、体系を作る部品。

### `README.md`

- **責務**：人の入口。何のリポジトリかと、`docs/README.md` への道を示す。
- **置くもの / 置かないもの**：数行の説明と、よく使う 2〜3 の文書へのリンク。文書の一覧は置かない。
- **当てはめた原則・パターン**：[段階的な開示](../reference/principles.md#progressive-disclosure)：最初に要るものだけ。
  [入口と唯一の一覧](../reference/patterns.md#single-index)：一覧は `docs/README.md` にだけ置く。

### `CLAUDE.md`

- **責務**：エージェントの入口。常に読み込まれる。
- **置くもの / 置かないもの**：ターゲットの契約にある節（プロダクトの前提、完了条件、言語など）と、`docs/README.md`
  への道。詳細は置かず、文書へのリンクにする。
- **関係**：節はターゲットの契約で決まる。
- **当てはめた原則・パターン**：[段階的な開示](../reference/principles.md#progressive-disclosure)：常に読み込まれるので前提と行き先
  だけ。[スキーマを先に](../reference/principles.md#schema-first)：節を決めて揃える。

### `docs/README.md`

- **責務**：そのリポジトリの文書の唯一の一覧。
- **置くもの / 置かないもの**：文書ごとに、種類と 1 行の説明。中身は置かない。
- **当てはめた原則・パターン**：[入口と唯一の一覧](../reference/patterns.md#single-index)。

### `docs/overview.md`

- **責務**：そのリポジトリの全体の解説。狙い、全体の構成と部分の間の関係、目指す姿。
- **置くもの / 置かないもの**：部分の細部は置かず、`architecture/` の各文書へリンクする。目指す姿は今の方針として
  書き、そこへの道筋（何をどの順でやるか）は Issue に置く。
- **当てはめた原則・パターン**：[関心の分離](../reference/principles.md#soc)：全体と部分を分ける。
  [判断の記録](../reference/patterns.md#adr)：道筋と理由はここに置かない。

### `docs/architecture/`

- **責務**：今の姿の解説。部分ごとに、その構成と、構成要素の説明。
- **置くもの / 置かないもの**：文書ごとに一つの部分（例：セッション、コードの層、devcontainer）。冒頭にその部分の
  構成と関係、続けて構成要素の説明を[形式](../reference/formats.md#component)で書く。選ばなかった案は置かず ADR に。
- **関係**：構成要素の説明から原則、パターン、ADR へリンクする。
- **当てはめた原則・パターン**：[文書の種類](../reference/patterns.md#diataxis)の解説。
  [判断の記録](../reference/patterns.md#adr)：ADR の連鎖をたどらなくても今の姿が分かるようにする。

### `docs/guides/`

- **責務**：手順。目的を果たすために何をどの順で行うか（準備、追加、移行、復旧）。
- **置くもの / 置かないもの**：手順と、その場で要る最小の説明。仕組みの説明は `architecture/` へリンクする。
  エージェントが使う手順は、スキルにしてもよい。
- **当てはめた原則・パターン**：[文書の種類](../reference/patterns.md#diataxis)の手順。

### `docs/reference/`

- **責務**：引くための事実。契約、設定、コマンド、形式、用語、決まり、原則の一覧。
- **置くもの / 置かないもの**：一覧や表。手順と理由は置かない。設定の既定値は、例の設定ファイルを正にする。
- **当てはめた原則・パターン**：[文書の種類](../reference/patterns.md#diataxis)の参照。
  [スキーマを先に](../reference/principles.md#schema-first)：契約と形式はここに先に書く。

### `docs/adr/`

- **責務**：判断の記録。
- **置くもの / 置かないもの**：[形式](../reference/formats.md#adr)に沿う。今の姿のまとめは置かない。
- **当てはめた原則・パターン**：[判断の記録](../reference/patterns.md#adr)、[メタデータ](../reference/patterns.md#metadata)：
  状態と参照を frontmatter に持たせて検査する。

### 原則の一覧（`reference/principles.md`）

- **責務**：何を大事にするかを、原則・何のため・一般的な当てはめ方・よくある違反で一か所に置く。
- **置くもの / 置かないもの**：どの粒度にも当てはまる一般的なことだけ。このプロジェクトのどこで使うかは置かない。
- **関係**：パターンと構成要素の説明から指される。ほかを指さない。
- **当てはめた原則・パターン**：[依存の向き](../reference/principles.md#dependency-rule)：いちばん内側で、何も前提にしない。
  [SSOT](../reference/principles.md#ssot)：定義はここだけ。

### パターンの一覧（`reference/patterns.md`）

- **責務**：よくある問題への解き方を、解く問題・形・使いどころ・よくある失敗・拠り所の原則で一か所に置く。
- **置くもの / 置かないもの**：一般的な形だけ。このプロジェクトのどこで使うかは置かない。
- **関係**：原則を拠り所として指す。構成要素の説明と design スキルから指される。
- **当てはめた原則・パターン**：[依存の向き](../reference/principles.md#dependency-rule)：原則だけを前提にする。

### 文書の体系（`architecture/documentation.md`、この文書）

- **責務**：このプロジェクト群の文書をどう分け、どこに何を書くかの解説。
- **置くもの / 置かないもの**：区切り、各リポジトリの器と、体系を作る部品の構成要素の説明。形の細部は形式に、
  書く手順は文書を書くに、理由は ADR に置く。
- **関係**：原則とパターンを指す。形式と手順を指す。共通のルールから指される。
- **当てはめた原則・パターン**：[文書の種類](../reference/patterns.md#diataxis)の解説。

### 形式（`reference/formats.md`）

- **責務**：構成要素の説明、ADR、リンクの形を決める。
- **関係**：check-docs.py がこの形を確かめる。
- **当てはめた原則・パターン**：[スキーマを先に](../reference/principles.md#schema-first)：形を先に決め、文書を合わせる。

### 文書を書く（`guides/write-docs.md`）

- **責務**：文書を足したり直したりするときの手順（区切り → 種類 → 持ち主 → 形 → 確かめる）。
- **当てはめた原則・パターン**：[文書の種類](../reference/patterns.md#diataxis)の手順。

### 用語集（`reference/glossary.md`）

- **責務**：区切りをまたぐ言葉の意味を一か所で定める。文書の体系の言葉（区切り、文書の種類、構成要素の説明など）も含む。
- **当てはめた原則・パターン**：[ユビキタス言語と用語集](../reference/principles.md#glossary)、
  [共有の核](../reference/patterns.md#shared-kernel)。

### 作業の進め方（`reference/workflow.md`）

- **責務**：変更（PR）と課題（Issue）の扱い方。文書に置かない道筋と予定の置き場所を決める。
- **当てはめた原則・パターン**：[今と経緯を分ける](../reference/principles.md#present-state)。

### 共通のルール（rules プラグイン）

- **責務**：いつも守らせたいことを、セッションとサブエージェントの始めに読み込ませる。
- **置くもの / 置かないもの**：コメント、コミット、PR に何を書くか（ここが持ち主）と、文書の決まりの短い要約。要約には
  どの原則や手順の要約かを書き、定義は原則とこの文書に任せる。
- **当てはめた原則・パターン**：[段階的な開示](../reference/principles.md#progressive-disclosure)：常に読まれるので短くする。
  [仕組みで守る](../reference/principles.md#mechanisms)：常に読み込む決まりの段。

### design スキル（review プラグイン）

- **責務**：どの変更にどのパターンを当てはめるかと、レビューでの指摘の仕方。
- **置くもの / 置かないもの**：パターンの定義は置かず、パターンの一覧を指す。
- **当てはめた原則・パターン**：[SSOT](../reference/principles.md#ssot)。

### 文書の検査（`tools/check-docs.py`）

- **責務**：形式と器の決まりのうち、機械で確かめられるものを `make check-docs` で確かめる。
- **置くもの / 置かないもの**：確かめるのは、相対リンクとアンカー、ADR の形と参照、`docs/` の器の構成、索引の抜け、
  構成要素の説明の責務。文の書き方や置き場所の良し悪しは確かめない（レビューの観点に任せる）。
- **関係**：形式に沿う。target-template の共通のファイルとして、どのリポジトリにも置く。
- **当てはめた原則・パターン**：[仕組みで守る](../reference/principles.md#mechanisms)、[Docs as Code](../reference/patterns.md#docs-as-code)。

## 決まりの強さ

決まりは、強いものから次の 4 つで守らせ、できるだけ強い方に寄せる（[仕組みで守る](../reference/principles.md#mechanisms)）。

| 強さ | 仕組み | 中身 |
| --- | --- | --- |
| 検査 | `make check-docs`（`tools/check-docs.py`） | リンクとアンカー、ADR の形と参照、`docs/` の器、索引の抜け、構成要素の責務（[形式](../reference/formats.md)の各節と、上の器） |
| 常に読み込む決まり | rules プラグイン | 一つひとつの文の書き方 |
| 必要なときに読む指針 | この文書、[形式](../reference/formats.md)、[文書を書く](../guides/write-docs.md)、review プラグインの観点 | 分け方、置き場所、形 |
| 判断の理由 | ADR | なぜそう決めたか |
