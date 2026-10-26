# 文書の体系

このプロジェクト群で、文書をどの区切りにどう置き、何が何を前提にするかと、決まりをどう定義して読み手（人と
エージェント）に届けるかの仕組み。ここは仕組みと、決まりのどの文書も前提にする横断の決まり（今の姿と理由の置き場所、
定義と届け方、振り分け）を持ち、決まりの細部は次の参照の文書と手順にある。

| 何の決まりか | 在りか |
| --- | --- |
| 文書の検査の範囲、文書の根と器、各文書の形（手順の層を含む）、リンク、量の上限、地図 | [文書の形式](../reference/formats.md "在りか") |
| 要約と案内、起点に何をどう載せるか、目的の手順の読ませ方 | [届け方の決まり](../reference/delivery.md "在りか") |
| 何を検査にし、検査をどこに置くか | [検査の扱い方](../reference/checks.md "在りか") |
| 文書を書く順 | [文書を書く](../guides/write-docs.md "在りか") |

判断の理由は ADR [0002](../adr/0002-documents-by-distribution-context.md)、[0003](../adr/0003-type-first-docs-and-platform-kernel.md)、[0005](../adr/0005-principles-and-conventions-plugins.md)、[0006](../adr/0006-nested-items-and-prerequisite-directions.md)、[0008](../adr/0008-maps-per-level-and-the-whole.md)、[0009](../adr/0009-definitions-and-delivery.md)、[0020](../adr/0020-scene-procedures-start-by-reading.md)、[0021](../adr/0021-purpose-procedures-and-triage.md)、[0039](../adr/0039-documentation-explains-and-references-hold-rules.md)、[0041](../adr/0041-reasons-by-kind.md) にある。
原則とパターンは[設計の原則](../../../principles/docs/reference/principles.md "在りか")と[設計のパターン](../../../principles/docs/reference/patterns.md "在りか")にある。

## 構成

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
  なる（扱いは[形式：地図](../reference/formats.md#map "在りか")）。
- 区切りの間の契約は、作る側にも使う側にも属さない独立した項目で、両側がそれを前提にする
  （[依存関係の逆転](../../../principles/docs/reference/principles.md#dip)）。置き場所の親と、関係の上での親は違ってよい。
- 項目の間の関係は種類で分け、依存の向きの決まりは「前提にする」関係にだけ当てる
  （[関係を種類で分ける](../../../principles/docs/reference/patterns.md#relation-kinds)）。在りかを示すだけのリンクは、依存に数えない。
- 前提の向きは階層ごとに決め、循環させない（[非循環依存](../../../principles/docs/reference/principles.md#acyclic)）。多くから前提にされる項目ほど
  変わりにくく小さく保つ（[安定依存](../../../principles/docs/reference/principles.md#stable-dependencies)）。二つの文書の関係は、
  それらが初めて同じ親の下に入る階層で、その親の下の項目どうしの向きとして判断する。
- 地図（どんな項目があり、どの項目がどれを前提にするか）は、階層ごとにその持ち主が書き、`check-docs` がすべての地図を
  集めて文書のリンクと照らす。地図からは[設計構造行列](../../../principles/docs/reference/patterns.md#dsm)を出せる。
  地図の置き場所と書き方、リンクの種類の書き方は[形式](../reference/formats.md "在りか")にある。

<a id="types"></a>

### 文書の根と種類

区切りごとに、索引を持つ `docs/` ディレクトリ（文書の根）を置き、その中を[文書の種類で分ける](../../../principles/docs/reference/patterns.md#diataxis)。
Diátaxis の 4 つの種類のうち解説、手順、参照を使い、チュートリアルのかわりに判断（ADR）を置く。文書の根と器ごとに
何を置くかは[形式：文書の根](../reference/formats.md#layout "在りか")にある。

| 種類 | 読み手の目的 | 置き場所 |
| --- | --- | --- |
| 解説 | 今の姿を理解する | `overview.md`、`architecture/` |
| 手順 | 目的を果たす | `guides/` |
| 参照 | 事実と決まりを引く | `reference/` |
| 判断 | 判断の理由をたどる | `adr/` |

文書の外の書きもののうち、コメント、コミットメッセージ、PR は[書き方](../reference/writing.md "在りか")に、Issue は
[課題の扱い方](../reference/issues.md "在りか")に、レビューの指摘と報告は[レビューの扱い方](../reference/reviews.md "在りか")に、
依頼への答えはその目的の手順にある。

<a id="reasons"></a>

### 今の姿と理由の置き場所

文書とコードのコメントには今の姿を書き、理由、道筋、経緯は種類ごとに次の所に置く（[今と経緯を分ける](../../../principles/docs/reference/principles.md#present-state)）。

| 書くもの | 置き場所 |
| --- | --- |
| 守らないと壊れる制約の理由（なぜその順か、なぜそれを避けるか） | 今の姿として、その制約のそばに一文で書く（コメント、解説、手順、参照） |
| 選んだ理由と、選ばなかった案 | ADR |
| 道筋と予定 | Issue |
| 変更の理由と経緯 | コミットメッセージと PR |

<a id="delivery"></a>

### 定義と届け方

決まりは、一か所で定義し、効き方の違う形でエージェントや人に届ける（[一つの出典から届ける](../../../principles/docs/reference/patterns.md#single-source)）。

- **定義**：決まりそのもの。原則とパターン（principles）、このプロジェクト群の決まりと手順（conventions の `docs/`）、
  区切りの間の契約。同じ決まりを二か所で定義しない。
- **届け方**：定義を、効く形にしたもの。新しい決まりを足さない。検査は定義を確かめ、ほかの届け方（共通のルール、
  役割ごとの指示、スキル）は定義を要約か案内で載せる。定義を変えたら、それを確かめる検査と、それを載せた届け方も
  直す。例外として、役割ごとの指示は役割に固有の振る舞いを、スキルはそのプラグインの道具の使い方（道具を使う手順を
  含む）を、自分で持ってよい。
- リポジトリの決まり（CLAUDE.md）が conventions の定義と食い違うときは、リポジトリの決まりに従う。

守らせたいことは、できるだけ強い届け方で届ける（[仕組みで守る](../../../principles/docs/reference/principles.md#mechanisms)）。
届け方の種類と効き方は下の構成要素に、それぞれに何をどう載せるかは[届け方の決まり](../reference/delivery.md "在りか")にある。

<a id="procedures"></a>

### 手順と役割

手順（`guides/`）は依頼の目的で切り、次の 2 層からなる（[階層的分解](../../../principles/docs/reference/principles.md#hierarchy)）。

| 層 | 何の手順か | 例 |
| --- | --- | --- |
| 目的の手順 | 依頼者が何をしてほしいかの単位。依頼を受けてから終えるまでの全体で、いくつものターンやセッションにわたることがある | 変更を届ける、レビューするなど |
| 共通の手順 | 目的の手順の段の中で使う、作業の一部分。いくつもの目的の手順から使いうる | 文書を書くなど |

依頼を受けたら、まずどの目的の手順に当たるかを決める（振り分け）。目的の手順のない依頼（事実を聞く質問など）は、依頼に
沿って進める。作業の途中で目的が変わったら（相談から変更へなど）、その目的の手順に入り直す。目的の手順は一つのターンで
終わるとは限らず（追加の依頼、レビューの指摘への答え、CI の失敗）、どこからを新しい依頼として振り分けから入るかは各目的の
手順に書く。続きのターンでは、今どの段にいるかを確かめてそこから進める。会話の記録がない新しいセッションで続きから
入るときと、ほかの作業の途中からその目的の手順に入るときは、「始めに読む」から始める。判断が要る点で依頼者に聞いて
待てないとき（待てるかは、セッションを動かすハーネスによる）は、妥当な方を選んで進め、その判断を答えに含める。

手順は役割（ロール）では切らない。役割は、権限と隔離の境界（書けるもの、残せる所）で、使うモデルも持つ。手順と役割は
多対多になり、どの目的の手順を受け持つかは役割ごとの指示の「受け持つ手順」に書く。振り分けと役割ごとの指示が食い違う
ときは、役割ごとの指示に従う。手順の形は[形式：手順](../reference/formats.md#procedure "在りか")に、「始めに読む」の段と
起点への並べ方は[届け方の決まり：目的の手順の読ませ方](../reference/delivery.md#procedures "在りか")にある。

## 構成要素

定義と、その届け方。届け方は、効き方の強いものから並べる。

### 定義の文書（principles と conventions の `docs/`、契約）

- **責務**：決まりと手順そのものを一か所で持ち、読み手が読みに行ったときに効く。
- **関係**：どの届け方からも前提にされる。手順は、起点とスキルから案内されて、当てはまる作業のときだけ読まれる。

### 検査（コマンド、試験、CI のジョブ、hook）

- **責務**：検査にした決まりを確かめ、守らないと通さない。
- **関係**：何を検査にし、どこに置くかは[検査の扱い方](../reference/checks.md "在りか")に、文書の検査が確かめるものは
  [形式](../reference/formats.md "在りか")の各節の「検査」にある。
- **当てはめた原則・パターン**：[仕組みで守る](../../../principles/docs/reference/principles.md#mechanisms)。

### 共通のルール（プラグインの `rules/`）

- **責務**：どの作業でも前提になる決まりを、セッションとサブエージェントの始めに読み込ませる。
- **関係**：何を載せるかは[届け方の決まり：起点](../reference/delivery.md#starting-points "在りか")にある。
- **当てはめた原則・パターン**：[段階的な開示](../../../principles/docs/reference/principles.md#progressive-disclosure)。

### 役割ごとの指示（プラグインの `agents/`、リポジトリの `.claude/agents/`）

- **責務**：その役割で動くときの前提（受け持つ手順、使う決まり、固有の振る舞い）を、作業の始めに読み込ませる。
- **関係**：形は[形式：役割ごとの指示](../reference/formats.md#role "在りか")にある。

### スキル（プラグインの `skills/`）

- **責務**：使うと決めたときに読み込まれ、手順への入口になり、そのプラグインの道具の使い方を持つ。
- **関係**：形は[形式：スキル](../reference/formats.md#skill "在りか")にある。
