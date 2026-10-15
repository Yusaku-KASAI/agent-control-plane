# 文書の体系

どのリポジトリでも、文書をどう分け、どこに何を書くか。判断の理由は
[ADR 0002](../adr/0002-documents-by-distribution-context.md)、[0003](../adr/0003-type-first-docs-and-platform-kernel.md)、[0005](../adr/0005-principles-and-conventions-plugins.md)、[0006](../adr/0006-nested-items-and-prerequisite-directions.md)、[0007](../adr/0007-navigation-links-marked-by-title.md)、[0008](../adr/0008-maps-per-level-and-the-whole.md)、[0009](../adr/0009-definitions-and-delivery.md)、[0010](../adr/0010-always-read-selection-and-size.md)、[0014](../adr/0014-prerequisites-by-change-and-whereabouts-links.md)、[0015](../adr/0015-starting-points-and-reading-scope.md) にある。
原則は[設計の原則](../../../principles/docs/reference/principles.md "在りか")、パターンは[設計のパターン](../../../principles/docs/reference/patterns.md "在りか")、形は[文書の形式](../reference/formats.md "在りか")、書くときの手順は
[文書を書く](../../skills/write-docs/SKILL.md "在りか")にある。

## 構成

文書の体系は、置き場所と関係（どこに何を置き、何が何を前提にするか）と、読み手への届け方（決まりを、どの文書に、
どう書いて読ませるか）の二つからなる。

### 置き場所と関係

#### 設計を記述する 3 つの層

| 層 | 書くもの | 置き場所 |
| --- | --- | --- |
| 原則とパターン | 原則（何を大事にするか）と、それを特定の問題への形にしたパターン（どう組むか）。一般的な当てはめ方とよくある失敗まで書き、どこで使うかは書かない | principles プラグインの [principles.md](../../../principles/docs/reference/principles.md "在りか") と [patterns.md](../../../principles/docs/reference/patterns.md "在りか") |
| アーキテクチャ | その区切りで採った全体の構成と、部分の間の関係 | 文書の根の `overview.md` と、`architecture/` の各文書の冒頭 |
| 構成要素の説明 | 層、ディレクトリ、ファイルごとの責務と関係、当てはめた原則・パターン | `architecture/` の各文書。[形式](../reference/formats.md#component)に沿う |

構成要素の説明から原則やパターンへリンクし、それがそこでどう効いているかを書く。一つの原則やパターンは多くの
構成要素に、一つの構成要素は多くの原則やパターンに当てはまる。

<a id="contexts"></a>

#### 区切りと依存の向き

- 知識は、使う人（誰が読めるか）と持ち主が同じものを一つの区切りにまとめる（[境界づけられた文脈](../../../principles/docs/reference/principles.md#bounded-context)）。
  区切りは最上位の項目で、それぞれが自分の文書の根を持つ。
- 依存は一方向にだけ向ける（[依存の向き](../../../principles/docs/reference/principles.md#dependency-rule)）。principles は何も前提にせず、
  conventions は principles だけを前提にする。この二つは、ほかのすべての区切りが前提にしてよい
  [共有の核](../../../principles/docs/reference/patterns.md#shared-kernel)になる。
- 共有の核のほかに区切りをまたぐのは、契約だけ。契約は使う側から読める場所に置き、作る側はそれに合わせて作り
  （[使う側に届く契約](../../../principles/docs/reference/patterns.md#published-language)）、実装との一致は[契約の試験](../../../principles/docs/reference/patterns.md#contract-test)で確かめる。

<a id="map"></a>

#### 項目の階層と関係

- 区切りと、その中の部品（プラグイン、話題の文書など）は、入れ子の項目として表す（[階層的分解](../../../principles/docs/reference/principles.md#hierarchy)）。
  何段あってもよい。同じ親の下の項目は、同じ種類と粒度にし、重ならず親を尽くす（[兄弟をそろえる](../../../principles/docs/reference/principles.md#siblings)）。
- 含む関係（親子）は依存ではない。子を持つ項目が直接持つ文書（`overview.md`、索引、全体の解説）は、子と並ぶ「全体」として
  扱う。子は全体（全体の構成や共通の考え方）を前提にしてよく、全体は子を前提にしない（子への道は在りかのリンクにする）。全体には
  全体についての事実だけを書き、子の細部を書き写さず、子へは[案内](#summaries-and-guides)で書く（[安定依存](../../../principles/docs/reference/principles.md#stable-dependencies)、
  [依存関係の逆転](../../../principles/docs/reference/principles.md#dip)）。
- 区切りの間の契約は、作る側にも使う側にも属さない独立した項目として扱い、両側がそれを前提にする
  （[依存関係の逆転](../../../principles/docs/reference/principles.md#dip)）。置き場所は使う側から読める所にする。置き場所の親と、関係の上での親は
  違ってよい。
- 項目の間の関係は種類で分け、依存の向きの決まりは「前提にする」関係にだけ当てる
  （[関係を種類で分ける](../../../principles/docs/reference/patterns.md#relation-kinds)）。在りかを示すだけのリンク（在りかのリンク）を、どう見分けてどう書くかは
  [形式](../reference/formats.md#links "在りか")にある。
- 前提の向きは階層ごとに決め、循環させない（[非循環依存](../../../principles/docs/reference/principles.md#acyclic)）。多くから前提にされる項目ほど
  変わりにくく小さく保つ（[安定依存](../../../principles/docs/reference/principles.md#stable-dependencies)）。二つの文書の関係は、それらが初めて同じ親の
  下に入る階層で、その親の下の項目どうしの向きとして判断する。下の階層の向きは、上の階層の向きの範囲の中でだけ決める。
- 地図（どんな項目があり、どの項目がどれを前提にするか）は、階層ごとにその持ち主が書く。最上位の地図は全体の設計を
  持つリポジトリの `overview.md` に、各項目の中の地図はその項目の `overview.md` に置き、上の階層の地図は中の項目を書かずに
  委ねる。書き方は[形式](../reference/formats.md#map "在りか")にあり、`check-docs` がすべての地図を集めて文書のリンクと照らす。
  地図からは[設計構造行列](../../../principles/docs/reference/patterns.md#dsm)を出せる。

<a id="roots"></a>

#### 文書の根と器

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

#### 文書の種類

[文書の種類で分ける](../../../principles/docs/reference/patterns.md#diataxis)のうち、解説、手順、参照を使い、チュートリアルは置かない（読み手が
限られ、手順で足りる）。かわりに判断（ADR）を 4 つ目の種類にする。

| 種類 | 置き場所 |
| --- | --- |
| 解説 | `overview.md`、`architecture/` |
| 手順 | `guides/`、プラグインのスキル |
| 参照 | `reference/` |
| 判断 | `adr/` |

#### 文書の外

コード中のコメント、コミットメッセージ、PR に何を書くかは[書き方](../reference/writing.md "在りか")にある。道筋と
予定は Issue に置き、文書には書かない（[課題の扱い方](../reference/issues.md "在りか")）。

<a id="rewrite"></a>

#### 書き換え

決まりを守るためなら、文書とコメントは破壊的に変えてよい。散らばった説明をまとめ直す、置き場所を移す、
ディレクトリ構成を変えることも含む。

### 読み手への届け方

<a id="delivery"></a>

#### 定義と届け方

決まりは、一か所で定義し、効き方の違う形でエージェントや人に届ける（[一つの出典から届ける](../../../principles/docs/reference/patterns.md#single-source)）。

- **定義**：決まりそのもの。原則とパターン（principles）、このプロジェクト群の決まり（conventions の `docs/`：書き方、
  変更と課題の扱い方、レビューの扱い方、文書の体系、形式、用語集）、区切りの間の契約。同じ決まりを二か所で定義しない。
- **届け方**：定義を、効く形に写したもの。新しい決まりを足さず、どの定義によるかを示す。定義を変えたら、写しも直す。
  例外として、役割ごとの指示は役割に固有の振る舞いを、スキルはそのプラグインの道具の使い方を、自分で持ってよい。
- リポジトリの決まり（CLAUDE.md）が conventions の定義と食い違うときは、リポジトリの決まりに従う。

定義がエージェントや人に効く形は、強いものから次のとおり。最後の行のほかが届け方で、最後の行は写しではなく、
定義の文書を読みに行くこと。守らせたいことは、できるだけ強い形で届ける（[仕組みで守る](../../../principles/docs/reference/principles.md#mechanisms)）。

| 形 | 効き方 | 例 |
| --- | --- | --- |
| 検査 | 守らないと通らない | `make check-docs`（何を確かめるかは[形式](../reference/formats.md "在りか")の各節の「検査」） |
| いつも読み込む要約 | 毎回の作業の前提になる。短くする | conventions の `rules/`（hook が読み込ませる） |
| 役割ごとの指示 | その役割で動くときの前提になる | プラグインの `agents/` の文書（どの決まりとスキルを使うかと、役割に固有の振る舞い） |
| 必要なときに読む手順と観点 | 当てはまる作業のときだけ読む | スキル（write-docs、design、review） |
| 定義の文書そのもの | 読みに行ったとき | principles と conventions の文書、契約 |

- **いつも読み込む要約**は、毎回すべての作業の前に読まれるので、載せるものを絞る。載せるのは、ほぼすべての作業に
  関わり、破られやすく、検査で守れない決まりだけ。特定の作業（文書を書く、変更する）にだけ要る決まりは、その作業の
  スキルや役割ごとの指示で届ける。
- 写しは、定義を[要約か案内](#summaries-and-guides)で書く。

<a id="summaries-and-guides"></a>

#### 要約と案内

文書がほかの文書の中身に触れるときは、次のどちらかで書く。

| 書き方 | 書くもの |
| --- | --- |
| 要約 | リンク先の中身を短く言い直したもの。開かなくても分かり、守れるようにする |
| 案内 | 中身を言い直さず、どの場面で、どこを読むかだけ |

- リンクの種類（前提のリンクか在りかのリンクか）は、これとは別に[形式](../reference/formats.md#links "在りか")の決め方で
  決める。要約が拠る文書（出典）へのリンクは、出典が変わると要約の見直しが要るので前提のリンクになる。案内のリンクは、
  ふつう在りかのリンクになる。
- 全体（`overview.md`、索引）から子へは案内で書き、子の細部を要約しない（[項目の階層と関係](#map)）。
- 一般的な原則と、それをこのプロジェクト群で具体化した決まりがあるときは、具体化した決まりを要約にし、原則はその
  理由として添える。

<a id="starting-points"></a>

#### 起点

起点は、エージェントがリンクをたどって選ぶのではなく、丸ごと読み込む文書。エージェントの入口（CLAUDE.md）、いつも
読み込む要約、役割ごとの指示は作業の始めに、スキルは使うと決めたときに読み込まれる。人の入口（`README.md`）、索引、
全体は、読み手が選んでたどる文書で、起点ではない。

書いてあっても読まれない、読まれても使われないことを避けるため、起点には載せるものを選ぶ。

- principles と conventions の定義を載せるときは、上のいつも読み込む要約に載せる基準を、その起点の範囲（すべての作業、
  その役割の作業、その作業）に当てはめて、要約にするか案内にするかを選ぶ。起点が自分で持つもの（CLAUDE.md の
  リポジトリの決まり、役割に固有の振る舞い、スキルの道具の使い方）は、この選び方に限らない。
- 判断の軸になる決まりを、細かい作法より先に載せる。読み手は先に書かれたものから読むためで、並び順で大事さを
  表すためではない。大事さは、起点に要約として書くことで表し、「大事」の印は付けない。
- 作業の始めに読み込まれる起点は、[形式](../reference/formats.md#size "在りか")の量の上限の中に収める
  （[必要十分](../../../principles/docs/reference/principles.md#sufficiency)）。
- 読み手に文書を全体まで読ませたいときは、起点に[読む範囲](#reading-scope)を書く。

<a id="reading-scope"></a>

#### 読む範囲

起点や、そこから読む定義の文書のリンク（役割ごとの指示では、パス）の直後に「（全体を読む）」を付けると、その場面ではリンク先の文書を全体まで読む。

- 全体を読んだ文書の中に、場面が当てはまる「（全体を読む）」があれば、それも読む。
- 付けないリンクは、要るときに、リンク先の節を読む。
- 「（全体を読む）」は、場面を限って付ける（例：決まりや文書を足す・移すとき）。いつも要るものは要約にするか、いつも
  読み込む要約に載せる。
- 読む範囲は、どれだけ読むかだけを表す。大事さは表さない。

## 構成要素

各文書の根の器の構成要素。体系を作る部品（conventions の定義と届け方、`tools/check-docs.py`）は
[conventions の全体像](../overview.md "在りか")にある。

### `README.md`

- **責務**：人の入口。何のリポジトリかと、索引（`docs/README.md`）と全体の解説（`docs/overview.md`、あれば）への道を示す。
- **置くもの / 置かないもの**：数行の説明、索引と全体の解説へのリンク、よく使う手順へのリンク（1〜2 本まで）。文書の
  一覧は置かない。
- **当てはめた原則・パターン**：[段階的な開示](../../../principles/docs/reference/principles.md#progressive-disclosure)：最初に要るものだけ。
  [入口と唯一の一覧](../../../principles/docs/reference/patterns.md#single-index)：一覧は `docs/README.md` にだけ置く。

### `CLAUDE.md`

- **責務**：エージェントの入口。常に読み込まれる。
- **置くもの / 置かないもの**：エージェントがいつも知っておくべき前提（プロダクトの前提、完了条件、言語など）と、索引
  （`docs/README.md`）と全体の解説（`docs/overview.md`、あれば）への道。詳細は置かず、文書へのリンクにする。
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
- **置くもの / 置かないもの**：一覧や表。決まりとして守る順序（変更の進め方、全体のレビューの手順）も置く。目的を果たすための操作の手順と、理由は置かない。設定の既定値は、例の設定ファイルを正にする。
- **当てはめた原則・パターン**：[文書の種類](../../../principles/docs/reference/patterns.md#diataxis)の参照。
  [スキーマを先に](../../../principles/docs/reference/principles.md#schema-first)：契約と形式はここに先に書く。

### `docs/adr/`

- **責務**：判断の記録。
- **置くもの / 置かないもの**：[形式](../reference/formats.md#adr)に沿う。今の姿のまとめは置かない。
- **当てはめた原則・パターン**：[判断の記録](../../../principles/docs/reference/patterns.md#adr)、[メタデータ](../../../principles/docs/reference/patterns.md#metadata)：
  状態と参照を frontmatter に持たせて検査する。
