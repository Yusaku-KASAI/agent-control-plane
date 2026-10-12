# 文書の体系

どのリポジトリでも、文書をどう分け、どこに何を書くか。判断の理由は
[ADR 0002](../adr/0002-documents-by-distribution-context.md)、[0003](../adr/0003-type-first-docs-and-platform-kernel.md)、[0005](../adr/0005-principles-and-conventions-plugins.md)、[0006](../adr/0006-nested-items-and-prerequisite-directions.md) にある。
原則は[設計の原則](../../../principles/docs/reference/principles.md)、パターンは[設計のパターン](../../../principles/docs/reference/patterns.md)、形は[文書の形式](../reference/formats.md)、書くときの手順は
[文書を書く](../../skills/write-docs/SKILL.md)にある。

## 構成

### 設計を記述する 3 つの層

| 層 | 書くもの | 置き場所 |
| --- | --- | --- |
| 原則とパターン | 原則（何を大事にするか）と、それを特定の問題への形にしたパターン（どう組むか）。一般的な当てはめ方とよくある失敗まで書き、どこで使うかは書かない | principles プラグインの [principles.md](../../../principles/docs/reference/principles.md) と [patterns.md](../../../principles/docs/reference/patterns.md) |
| アーキテクチャ | その区切りで採った全体の構成と、部分の間の関係 | 文書の根の `overview.md` と、`architecture/` の各文書の冒頭 |
| 構成要素の説明 | 層、ディレクトリ、ファイルごとの責務と関係、当てはめた原則・パターン | `architecture/` の各文書。[形式](../reference/formats.md#component)に沿う |

構成要素の説明から原則やパターンへリンクし、それがそこでどう効いているかを書く。一つの原則やパターンは多くの
構成要素に、一つの構成要素は多くの原則やパターンに当てはまる。

### 区切りと依存の向き

- 知識は、使う人（誰が読めるか）と持ち主が同じものを一つの区切りにまとめる（[境界づけられた文脈](../../../principles/docs/reference/principles.md#bounded-context)）。
  区切りは、リポジトリか、リポジトリの中で独立して配る単位（プラグインなど）に当て、それぞれが自分の文書の根を持つ。
- 依存は一方向にだけ向ける（[依存の向き](../../../principles/docs/reference/principles.md#dependency-rule)）。principles は何も前提にせず、
  conventions は principles だけを前提にする。この二つは、ほかのすべての区切りが前提にしてよい
  [共有の核](../../../principles/docs/reference/patterns.md#shared-kernel)になる。
- 共有の核のほかに区切りをまたぐのは、契約だけ。契約は使う側から読める場所に置き、作る側はそれに合わせて作り
  （[使う側に届く契約](../../../principles/docs/reference/patterns.md#published-language)）、実装との一致は[契約の試験](../../../principles/docs/reference/patterns.md#contract-test)で確かめる。

<a id="map"></a>

### 項目の階層と関係

- 区切りと、その中の部品（プラグイン、話題の文書など）は、入れ子の項目として表す（[階層的分解](../../../principles/docs/reference/principles.md#hierarchy)）。
  何段あってもよい。同じ親の下の項目は、同じ種類と粒度にし、重ならず親を尽くす（[兄弟をそろえる](../../../principles/docs/reference/principles.md#siblings)）。
- 区切りの間の契約は、作る側にも使う側にも属さない独立した項目として扱い、両側がそれを前提にする
  （[依存関係の逆転](../../../principles/docs/reference/principles.md#dip)）。置き場所は使う側から読める所にする。置き場所の親と、関係の上での親は
  違ってよい。
- 項目の間の関係は種類で分ける（[関係を種類で分ける](../../../principles/docs/reference/patterns.md#relation-kinds)）。依存の向きの決まりは「前提にする」
  関係にだけ当てる。索引（`docs/README.md`）から各文書へのリンクと、ADR（判断の理由）へのリンクは案内で、依存ではない。
  前提にされる側から前提にする側の在りかを示したいときは、リンクにせず文章で書く。
- 前提の向きは階層ごとに決め、循環させない（[非循環依存](../../../principles/docs/reference/principles.md#acyclic)）。多くから前提にされる項目ほど
  変わりにくく小さく保つ（[安定依存](../../../principles/docs/reference/principles.md#stable-dependencies)）。二つの文書の関係は、それらが初めて同じ親の
  下に入る階層で、その親の下の項目どうしの向きとして判断する。下の階層の向きは、上の階層の向きの範囲の中でだけ決める。
- 地図（どんな項目があり、どの項目がどれを前提にするか）は階層ごとに書く。最上位の地図は全体の設計を持つリポジトリの
  `overview.md` に、各項目の中の地図はその項目の `overview.md`（なければ構成を書いた文書）に置く。項目が多いときは
  [設計構造行列](../../../principles/docs/reference/patterns.md#dsm)で表す。

### 文書の根と器

区切りごとに一つ、`README.md` を持つ `docs/` ディレクトリ（文書の根）を置く。リポジトリ自身の文書の根はルートの
`docs/`、リポジトリの中のほかの区切り（プラグインなど）の文書の根は、その区切りのディレクトリの `docs/`。
入口（`README.md` と `CLAUDE.md`）はリポジトリのルートにだけ置く。

```
README.md
CLAUDE.md
docs/                 （区切りのディレクトリの中なら <区切り>/docs/）
  README.md
  overview.md
  architecture/
  guides/
  reference/
  adr/
```

- 要らないものは置かない。小さな区切り（プラグインなど）は、`README.md` と `overview.md`（構成と構成要素）だけでもよい。
- プラグインでは、エージェントが使う手順は `guides/` ではなくスキル（`skills/`）にする。
- 文書の根の中で関心の違う部分は、`architecture/` と `adr/` を `<関心>/` に分ける。`guides/` と `reference/` は、
  文書が増えたら同じように分ける。

<a id="types"></a>

### 文書の種類

[文書の種類で分ける](../../../principles/docs/reference/patterns.md#diataxis)のうち、解説、手順、参照を使い、チュートリアルは置かない（読み手が
限られ、手順で足りる）。かわりに判断（ADR）を 4 つ目の種類にする。

| 種類 | 置き場所 |
| --- | --- |
| 解説 | `overview.md`、`architecture/` |
| 手順 | `guides/`、プラグインのスキル |
| 参照 | `reference/` |
| 判断 | `adr/` |

### 文書の外

コード中のコメント、コミットメッセージ、PR に何を書くかは、共通のルール（conventions の `rules/`）にある。道筋と
予定は Issue に置き、文書には書かない（[作業の進め方](../reference/workflow.md)）。

### 書き換え

決まりを守るためなら、文書とコメントは破壊的に変えてよい。散らばった説明をまとめ直す、置き場所を移す、
ディレクトリ構成を変えることも含む。

### 決まりの強さ

決まりは、強いものから次の 3 つで守らせ、できるだけ強い方に寄せる（[仕組みで守る](../../../principles/docs/reference/principles.md#mechanisms)）。
なぜそう決めたかは ADR にある。

| 強さ | 仕組み |
| --- | --- |
| 検査 | `make check-docs`（何を確かめるかは[形式](../reference/formats.md)の各節の「検査」） |
| 常に読み込む決まり | conventions の `rules/`（hook が読み込ませる） |
| 必要なときに読む指針 | この文書、[形式](../reference/formats.md)、write-docs と design のスキル |

### 体系を作る部品

体系は、各リポジトリが持つ上の器と、それを決めて守らせる principles と conventions の部品からなる。

```
principles：原則（principles.md）◀──拠り所──パターン（patterns.md）
                  ▲                              ▲
                  └──── 構成要素の説明がリンクする ┘
principles スキル ──引く──▶ 原則、パターン
conventions：
  文書の体系（この文書）──形──▶ 形式（formats.md）◀──確かめる── check-docs.py（各リポジトリ）
     │
     └──手順──▶ write-docs スキル
  rules ＝ 常に読み込む要約 ──指す──▶ 原則、この文書
  hook（scripts/context）──読み込ませる──▶ rules と、文書の在りか
  design スキル ──どの変更にどのパターンか──▶ パターン
  用語集（glossary.md）：文書と作業の言葉
```

## 構成要素

器の構成要素（各文書の根にある）と、体系を作る部品（principles と conventions のプラグイン、各リポジトリの
`tools/check-docs.py`）。

### `README.md`

- **責務**：人の入口。何のリポジトリかと、`docs/README.md` への道を示す。
- **置くもの / 置かないもの**：数行の説明と、よく使う 2〜3 の文書へのリンク。文書の一覧は置かない。
- **当てはめた原則・パターン**：[段階的な開示](../../../principles/docs/reference/principles.md#progressive-disclosure)：最初に要るものだけ。
  [入口と唯一の一覧](../../../principles/docs/reference/patterns.md#single-index)：一覧は `docs/README.md` にだけ置く。

### `CLAUDE.md`

- **責務**：エージェントの入口。常に読み込まれる。
- **置くもの / 置かないもの**：エージェントがいつも知っておくべき前提（プロダクトの前提、完了条件、言語など）と、
  `docs/README.md` への道。詳細は置かず、文書へのリンクにする。
- **関係**：どの節を持つかは、リポジトリが従う契約で決める。
- **当てはめた原則・パターン**：[段階的な開示](../../../principles/docs/reference/principles.md#progressive-disclosure)：常に読み込まれるので前提と行き先
  だけ。[スキーマを先に](../../../principles/docs/reference/principles.md#schema-first)：節を決めて揃える。

### `docs/README.md`

- **責務**：そのリポジトリの文書の唯一の一覧。
- **置くもの / 置かないもの**：文書ごとに、種類と 1 行の説明。中身は置かない。
- **当てはめた原則・パターン**：[入口と唯一の一覧](../../../principles/docs/reference/patterns.md#single-index)。

### `docs/overview.md`

- **責務**：そのリポジトリの全体の解説。狙い、全体の構成と部分の間の関係、目指す姿。
- **置くもの / 置かないもの**：部分の細部は置かず、`architecture/` の各文書へリンクする。目指す姿は今の方針として
  書き、そこへの道筋（何をどの順でやるか）は Issue に置く。
- **当てはめた原則・パターン**：[関心の分離](../../../principles/docs/reference/principles.md#soc)：全体と部分を分ける。
  [判断の記録](../../../principles/docs/reference/patterns.md#adr)：道筋と理由はここに置かない。

### `docs/architecture/`

- **責務**：今の姿の解説。部分ごとに、その構成と、構成要素の説明。
- **置くもの / 置かないもの**：文書ごとに一つの部分（例：セッション、コードの層、devcontainer）。冒頭にその部分の
  構成と関係、続けて構成要素の説明を[形式](../reference/formats.md#component)で書く。選ばなかった案は置かず ADR に。
- **関係**：構成要素の説明から原則、パターン、ADR へリンクする。
- **当てはめた原則・パターン**：[文書の種類](../../../principles/docs/reference/patterns.md#diataxis)の解説。
  [判断の記録](../../../principles/docs/reference/patterns.md#adr)：ADR の連鎖をたどらなくても今の姿が分かるようにする。

### `docs/guides/`

- **責務**：手順。目的を果たすために何をどの順で行うか（準備、追加、移行、復旧）。
- **置くもの / 置かないもの**：手順と、その場で要る最小の説明。仕組みの説明は `architecture/` へリンクする。
  エージェントが使う手順は、スキルにしてもよい。
- **当てはめた原則・パターン**：[文書の種類](../../../principles/docs/reference/patterns.md#diataxis)の手順。

### `docs/reference/`

- **責務**：引くための事実。契約、設定、コマンド、形式、用語、決まり、原則の一覧。
- **置くもの / 置かないもの**：一覧や表。手順と理由は置かない。設定の既定値は、例の設定ファイルを正にする。
- **当てはめた原則・パターン**：[文書の種類](../../../principles/docs/reference/patterns.md#diataxis)の参照。
  [スキーマを先に](../../../principles/docs/reference/principles.md#schema-first)：契約と形式はここに先に書く。

### `docs/adr/`

- **責務**：判断の記録。
- **置くもの / 置かないもの**：[形式](../reference/formats.md#adr)に沿う。今の姿のまとめは置かない。
- **当てはめた原則・パターン**：[判断の記録](../../../principles/docs/reference/patterns.md#adr)、[メタデータ](../../../principles/docs/reference/patterns.md#metadata)：
  状態と参照を frontmatter に持たせて検査する。

### 原則の一覧（principles の `docs/reference/principles.md`）

- **責務**：何を大事にするかを、原則・何のため・一般的な当てはめ方・よくある違反で一か所に置く。
- **置くもの / 置かないもの**：どの粒度にも当てはまる一般的なことだけ。このプロジェクトのどこで使うかは置かない。
- **関係**：パターンと構成要素の説明から指される。ほかを指さない。
- **当てはめた原則・パターン**：[依存の向き](../../../principles/docs/reference/principles.md#dependency-rule)：いちばん内側で、何も前提にしない。
  [SSOT](../../../principles/docs/reference/principles.md#ssot)：定義はここだけ。

### パターンの一覧（principles の `docs/reference/patterns.md`）

- **責務**：よくある問題への解き方を、解く問題・形・使いどころ・よくある失敗・拠り所の原則で一か所に置く。
- **置くもの / 置かないもの**：一般的な形だけ。このプロジェクトのどこで使うかは置かない。
- **関係**：原則を拠り所として指す。構成要素の説明と design スキルから指される。
- **当てはめた原則・パターン**：[依存の向き](../../../principles/docs/reference/principles.md#dependency-rule)：原則だけを前提にする。

### 文書の体系（`docs/architecture/documentation.md`、この文書）

- **責務**：このプロジェクト群の文書をどう分け、どこに何を書くかの解説。
- **置くもの / 置かないもの**：区切り、各リポジトリの器と、体系を作る部品の構成要素の説明。形の細部は形式に、
  書く手順は文書を書くに、理由は ADR に置く。
- **関係**：原則とパターンを指す。形式と手順を指す。共通のルールから指される。
- **当てはめた原則・パターン**：[文書の種類](../../../principles/docs/reference/patterns.md#diataxis)の解説。

### 形式（`docs/reference/formats.md`）

- **責務**：構成要素の説明、ADR、リンクの形を決める。
- **関係**：check-docs.py がこの形を確かめる。
- **当てはめた原則・パターン**：[スキーマを先に](../../../principles/docs/reference/principles.md#schema-first)：形を先に決め、文書を合わせる。

### 文書を書く（`skills/write-docs/`）

- **責務**：文書を足したり直したりするときの手順（区切り → 種類 → 持ち主 → 形 → 確かめる）。
- **当てはめた原則・パターン**：[文書の種類](../../../principles/docs/reference/patterns.md#diataxis)の手順。

### 用語集（`docs/reference/glossary.md`）

- **責務**：文書の体系と作業の進め方の言葉（区切り、文書の種類、構成要素の説明など）を一か所で定める。ほかの言葉は、
  それを使う区切りの用語集にある。
- **当てはめた原則・パターン**：[ユビキタス言語と用語集](../../../principles/docs/reference/principles.md#glossary)、
  [共有の核](../../../principles/docs/reference/patterns.md#shared-kernel)。

### 作業の進め方（`docs/reference/workflow.md`）

- **責務**：変更（PR）と課題（Issue）の扱い方。文書に置かない道筋と予定の置き場所を決める。
- **当てはめた原則・パターン**：[今と経緯を分ける](../../../principles/docs/reference/principles.md#present-state)。

### 共通のルール（`rules/`）

- **責務**：いつも守らせたいことを、セッションとサブエージェントの始めに読み込ませる。
- **置くもの / 置かないもの**：コメント、コミット、PR に何を書くか（ここが持ち主）と、文書の決まりの短い要約。要約には
  どの原則や手順の要約かを書き、定義は原則とこの文書に任せる。
- **当てはめた原則・パターン**：[段階的な開示](../../../principles/docs/reference/principles.md#progressive-disclosure)：常に読まれるので短くする。
  [仕組みで守る](../../../principles/docs/reference/principles.md#mechanisms)：常に読み込む決まりの段。

### hook（`hooks/`、`scripts/context`）

- **責務**：セッションとサブエージェントの始めに、共通のルールと、conventions と principles の文書の在りかを読み込ませる。
- **関係**：文書の在りかは、このプラグインの場所と、同じマーケットプレイスにある principles の場所から決める。
- **当てはめた原則・パターン**：[段階的な開示](../../../principles/docs/reference/principles.md#progressive-disclosure)：中身ではなく在りかを渡す。

### principles スキル（principles の `skills/principles/`）

- **責務**：原則とパターンの一覧を、要るときに引けるようにする。
- **置くもの / 置かないもの**：一覧の在りかと引き方だけ。原則とパターンの中身は置かない。

### design スキル（`skills/design/`）

- **責務**：どの変更にどのパターンを当てはめるかと、レビューでの指摘の仕方。
- **置くもの / 置かないもの**：パターンの定義は置かず、パターンの一覧を指す。
- **当てはめた原則・パターン**：[SSOT](../../../principles/docs/reference/principles.md#ssot)。

### 文書の検査（`tools/check-docs.py`）

- **責務**：形式と器の決まりのうち、機械で確かめられるものを `make check-docs` で確かめる。
- **置くもの / 置かないもの**：何を確かめるかは[形式](../reference/formats.md)の各節の「検査」にある。文の書き方や置き場所の
  良し悪しは確かめない（レビューの観点に任せる）。
- **関係**：形式に沿う。どのリポジトリにも同じものを置く。
- **当てはめた原則・パターン**：[仕組みで守る](../../../principles/docs/reference/principles.md#mechanisms)、[Docs as Code](../../../principles/docs/reference/patterns.md#docs-as-code)。
