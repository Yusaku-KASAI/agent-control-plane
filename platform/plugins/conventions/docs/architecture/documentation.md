# 文書の体系

このプロジェクト群で、文書をどの区切りにどう置き、何が何を前提にするかと、決まりをどう定義して読み手（人と
エージェント）に届けるかの仕組み。ここは仕組みだけを解説し、決まりの細部は次の参照の文書と手順にある。

| 何の決まりか | 在りか |
| --- | --- |
| 文書の根と器、各文書の形、リンク、量の上限、地図 | [文書の形式](../reference/formats.md "在りか") |
| 要約と案内、起点に何をどう載せるか、目的の手順と共通の手順 | [届け方](../reference/delivery.md "在りか") |
| 何を検査にし、検査をどこに置くか | [検査の扱い方](../reference/checks.md "在りか") |
| 文書を書く順 | [文書を書く](../guides/write-docs.md "在りか") |

判断の理由は ADR [0002](../adr/0002-documents-by-distribution-context.md)、[0003](../adr/0003-type-first-docs-and-platform-kernel.md)、[0006](../adr/0006-nested-items-and-prerequisite-directions.md)、[0008](../adr/0008-maps-per-level-and-the-whole.md)、[0009](../adr/0009-definitions-and-delivery.md)、[0020](../adr/0020-scene-procedures-start-by-reading.md)、[0039](../adr/0039-documentation-explains-and-references-hold-rules.md) にある。
原則とパターンは[設計の原則](../../../principles/docs/reference/principles.md "在りか")と[設計のパターン](../../../principles/docs/reference/patterns.md "在りか")にある。

## 構成

文書の体系は、置き場所と関係（どこに何を置き、何が何を前提にするか）と、読み手への届け方（決まりを、どの文書に、
どう書いて読ませるか）の二つのまとまりからなる。

### 設計を記述する 3 つの層

| 層 | 書くもの | 置き場所 |
| --- | --- | --- |
| 原則とパターン | 原則（何を大事にするか）と、それを特定の問題への形にしたパターン（どう組むか）。一般的な当てはめ方とよくある失敗まで書き、どこで使うかは書かない | principles プラグインの [principles.md](../../../principles/docs/reference/principles.md "在りか") と [patterns.md](../../../principles/docs/reference/patterns.md "在りか") |
| アーキテクチャ | その区切りで採った全体の構成と、部分の間の関係 | 文書の根の `overview.md` と、`architecture/` の各文書の冒頭 |
| 構成要素の説明 | 層、ディレクトリ、ファイルごとの責務と関係、当てはめた原則・パターン | `architecture/` の各文書。[形式](../reference/formats.md#component "在りか")に沿う |

構成要素の説明から原則やパターンへリンクし、それがそこでどう効いているかを書く。一つの原則やパターンは多くの
構成要素に、一つの構成要素は多くの原則やパターンに当てはまる。

<a id="contexts"></a>

### 区切りと依存の向き

- 知識は、使う人（誰が読めるか）と持ち主が同じものを一つの区切りにまとめる（[境界づけられた文脈](../../../principles/docs/reference/principles.md#bounded-context)）。
  区切りは最上位の項目で、それぞれが自分の文書の根を持つ。
- 依存は一方向にだけ向ける（[依存の向き](../../../principles/docs/reference/principles.md#dependency-rule)）。principles は何も前提にせず、
  conventions は principles だけを前提にする。この二つは、ほかのすべての区切りが前提にしてよい
  [共有の核](../../../principles/docs/reference/patterns.md#shared-kernel)になる。
- 共有の核のほかに区切りをまたぐのは、契約だけ。契約は使う側から読める場所に置き、作る側はそれに合わせて作り
  （[使う側に届く契約](../../../principles/docs/reference/patterns.md#published-language)）、実装との一致は[契約の試験](../../../principles/docs/reference/patterns.md#contract-test)で確かめる。

<a id="map"></a>

### 項目の階層と地図

- 区切りと、その中の部品（プラグイン、話題の文書など）は、入れ子の項目として表す（[階層的分解](../../../principles/docs/reference/principles.md#hierarchy)）。
  何段あってもよい。同じ親の下の項目は、同じ種類と粒度にし、重ならず親を尽くす（[兄弟をそろえる](../../../principles/docs/reference/principles.md#siblings)）。
- 含む関係（親子）は依存ではない。子を持つ項目が直接持つ文書（`overview.md`、索引、全体の解説）は、子と並ぶ「全体」に
  なる。全体は子から前提にされ、子を前提にしない（[安定依存](../../../principles/docs/reference/principles.md#stable-dependencies)、
  [依存関係の逆転](../../../principles/docs/reference/principles.md#dip)）。
- 区切りの間の契約は、作る側にも使う側にも属さない独立した項目で、両側がそれを前提にする
  （[依存関係の逆転](../../../principles/docs/reference/principles.md#dip)）。置き場所の親と、関係の上での親は違ってよい。
- 項目の間の関係は種類で分け、依存の向きの決まりは「前提にする」関係にだけ当てる
  （[関係を種類で分ける](../../../principles/docs/reference/patterns.md#relation-kinds)）。在りかを示すだけのリンクは、依存に数えない。
- 前提の向きは階層ごとに決め、循環させない（[非循環依存](../../../principles/docs/reference/principles.md#acyclic)）。多くから前提にされる項目ほど
  変わりにくく小さく保つ。二つの文書の関係は、それらが初めて同じ親の下に入る階層で、その親の下の項目どうしの
  向きとして判断する。
- 地図（どんな項目があり、どの項目がどれを前提にするか）は、階層ごとにその持ち主が書き、`check-docs` がすべての地図を
  集めて文書のリンクと照らす。地図からは[設計構造行列](../../../principles/docs/reference/patterns.md#dsm)を出せる。
  地図の置き場所、全体の扱い、リンクの種類の書き方は[形式](../reference/formats.md "在りか")にある。

<a id="types"></a>

### 文書の根と種類

区切りごとに一つ、索引（`README.md`）を持つ `docs/` ディレクトリ（文書の根）を置く。入口（`README.md` と
`CLAUDE.md`）はリポジトリのルートにだけ置く。文書の根の中は、[文書の種類で分ける](../../../principles/docs/reference/patterns.md#diataxis)のうち
解説、手順、参照を使い、チュートリアルは置かない（読み手が限られ、手順で足りる）。かわりに判断（ADR）を 4 つ目の
種類にする。

| 種類 | 書くもの | 置き場所 |
| --- | --- | --- |
| 解説 | 今の姿：全体と部分の構成、構成要素の説明 | `overview.md`、`architecture/` |
| 手順 | 目的を果たすために、何をどの順で行うか | `guides/` |
| 参照 | 引くための事実と決まり：契約、設定、コマンド、形式、用語、条件、表 | `reference/` |
| 判断 | 判断の記録：文脈、決めたこと、選ばなかった案 | `adr/` |

器ごとに何を置くかは[形式：文書の根](../reference/formats.md#layout "在りか")にある。文書の外の書きもののうち、
コメント、コミットメッセージ、PR は[書き方](../reference/writing.md "在りか")に、Issue は[課題の扱い方](../reference/issues.md "在りか")に、
レビューの指摘と報告は[レビューの扱い方](../reference/reviews.md "在りか")に、依頼への答えはその目的の手順にある。
道筋と予定は Issue に置き、文書には書かない。

<a id="delivery"></a>

### 定義と届け方

決まりは、一か所で定義し、効き方の違う形でエージェントや人に届ける（[一つの出典から届ける](../../../principles/docs/reference/patterns.md#single-source)）。

- **定義**：決まりそのもの。原則とパターン（principles）、このプロジェクト群の決まり（conventions の `docs/`）、区切りの
  間の契約。同じ決まりを二か所で定義しない。
- **届け方**：定義を、効く形にしたもの。新しい決まりを足さず、定義を要約か案内で載せる。定義を変えたら、それを
  確かめる検査と、それを載せた届け方も直す。例外として、役割ごとの指示は役割に固有の振る舞いを、スキルはその
  プラグインの道具の使い方を、自分で持ってよい。
- リポジトリの決まり（CLAUDE.md）が conventions の定義と食い違うときは、リポジトリの決まりに従う。

届け方の形と、それぞれの効き方は、下の構成要素にある。守らせたいことは、できるだけ強い形で届ける
（[仕組みで守る](../../../principles/docs/reference/principles.md#mechanisms)）。各形に何をどう載せるかは
[届け方](../reference/delivery.md "在りか")にある。

<a id="procedures"></a>

### 手順と役割

手順（`guides/`）は、依頼者が何をしてほしいかの単位の目的の手順（変更を届けるなど）と、その段の中で使う共通の手順
（文書を書くなど）の 2 層からなる。依頼を受けたら、まずどの目的の手順に当たるかを決める（振り分け）。役割（ロール）は
権限と隔離の境界で、手順とは多対多になり、どの目的の手順を受け持つかは役割ごとの指示に書く。層、振り分け、起点への
並べ方の決まりは[届け方：目的の手順と共通の手順](../reference/delivery.md#procedures "在りか")にある。

## 構成要素

定義と、定義を届ける形。届ける形は、効き方の強いものから並べる。

### 定義の文書（principles と conventions の `docs/`、契約）

- **責務**：決まりそのものを一か所で持つ。読み手が読みに行ったときに効く。
- **関係**：届ける形のすべてから前提にされる。

### 検査（conventions の `check-docs`、各リポジトリの検査）

- **責務**：検査にした決まりを確かめ、守らないと通さない。最も強いが、保つ手間がかかり、誤って落ちると作業を止める。
- **置くもの / 置かないもの**：何を検査にし、どこに置くかは[検査の扱い方](../reference/checks.md "在りか")に、文書の検査が
  確かめるものは[形式](../reference/formats.md "在りか")の各節の「検査」にある。
- **当てはめた原則・パターン**：[仕組みで守る](../../../principles/docs/reference/principles.md#mechanisms)。

### 共通のルール（conventions の `rules/`）

- **責務**：どの作業でも前提になる決まりを、セッションとサブエージェントの始めに読み込ませる（hook が読み込ませる）。
- **置くもの / 置かないもの**：どの作業でも全体まで読む文書（この文書）、振り分け、ほぼすべての作業に関わる決まりの
  要約と案内。特定の作業にだけ要る決まりは置かない。
- **当てはめた原則・パターン**：[段階的な開示](../../../principles/docs/reference/principles.md#progressive-disclosure)：毎回読まれるので短くする。

### 役割ごとの指示（プラグインの `agents/`、リポジトリの `.claude/agents/`）

- **責務**：その役割で動くときの前提を、作業の始めに読み込ませる。
- **置くもの / 置かないもの**：受け持つ手順（書ける範囲と、目的の手順ごとの扱い）、使う決まりの要約と案内、役割に固有の
  振る舞い。形は[形式：役割ごとの指示](../reference/formats.md#role "在りか")にある。

### スキル（プラグインの `skills/`）

- **責務**：使うと決めたときに読み込まれ、手順への入口になる。そのプラグインの道具の使い方を持つ。
- **置くもの / 置かないもの**：届ける手順への案内と、破られやすいことの要約、道具の使い方。手順そのものは置かない。

### 手順（`guides/`）

- **責務**：決まりに沿って作業する順を持つ。起点（共通のルール、役割ごとの指示）とスキルから届き、当てはまる作業の
  ときだけ読まれる。
- **置くもの / 置かないもの**：段と、その場で要る最小の説明。決まり（条件、表、形）は参照の文書に置き、段からリンクする。
