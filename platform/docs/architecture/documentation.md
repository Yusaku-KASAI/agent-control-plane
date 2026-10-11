# 文書の体系

どのリポジトリでも、文書をどう分け、どこに何を書くか。判断の理由は
[ADR 0005](../adr/0005-documents-by-distribution-context.md) と [ADR 0007](../adr/0007-type-first-docs-and-platform-kernel.md) にある。
原則は[設計の原則](../reference/principles.md)、形は[文書の形式](../reference/formats.md)、書くときの手順は
[文書を書く](../guides/write-docs.md)にある。

## 全体の構成

### 設計を記述する 3 つの層

| 層 | 書くもの | 置き場所 |
| --- | --- | --- |
| 原則 | 原則そのものと一般的な当てはめ方。どこで使うかは書かない | platform の [reference/principles.md](../reference/principles.md) |
| アーキテクチャ | そのリポジトリで採った全体の構成と、部分の間の関係 | 各リポジトリの `docs/overview.md` と、`docs/architecture/` の各文書の冒頭 |
| 構成要素の説明 | 層、ディレクトリ、ファイルごとの責務と関係、当てはめた原則 | `docs/architecture/` の各文書。[形式](../reference/formats.md#component)に沿う |

構成要素の説明から原則へリンクし、その原則がそこでどう効いているかを書く。一つの原則は多くの構成要素に、
一つの構成要素は多くの原則に当てはまる。

### 区切りと依存の向き

知識は、使う人（誰が読めるか）と持ち主が同じものを一つの区切りにまとめ、区切りごとにリポジトリを分ける。
切り出すまでは、このリポジトリに 3 つの区切りが同居する。

| 区切り | 置くもの | 届く先 | 今の場所 |
| --- | --- | --- | --- |
| platform | 原則、文書の体系と形式、作業の進め方、ハーネスの契約、用語集、ロール、スキル | すべてのエージェント（マーケットプレイスと一緒に `docs/` が届く） | `platform/` |
| target-template | ターゲットの契約、devcontainer、CI | すべてのターゲット（テンプレートから作る） | `template/docs/` と、ルートの環境のファイル |
| control-plane | 全体の設計、アプリの仕組み、Go の書き方、運用の手順 | control-plane の開発者 | `docs/` |
| 各ターゲット | プロダクトの前提、そのリポジトリの仕組みと判断 | そのターゲットのエージェント | 各リポジトリ |

- platform は、すべての区切りに届く共有の核（Shared Kernel）。ほかの区切りは platform の文書を前提にしてよい。
  platform はほかの区切りの文書を前提にしない（[依存の向き](../reference/principles.md#dependency-rule)）。
- platform 以外の区切りの間をつなぐのは、契約だけ。ターゲットの契約（target-template にある）と、ハーネスの契約
  （使うエージェントに届くよう platform にある）。契約は使う側から読める場所に置き、作る側はそれに合わせて作る。

## 構成要素

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

### `README.md`

- **責務**：人の入口。何のリポジトリかと、`docs/README.md` への道を示す。
- **置くもの / 置かないもの**：数行の説明と、よく使う 2〜3 の文書へのリンク。文書の一覧は置かない。
- **当てはめた原則**：[段階的な開示](../reference/principles.md#progressive-disclosure)：最初に要るものだけ。
  [SSOT](../reference/principles.md#ssot)：一覧は `docs/README.md` にだけ置く。

### `CLAUDE.md`

- **責務**：エージェントの入口。常に読み込まれる。
- **置くもの / 置かないもの**：ターゲットの契約にある節（プロダクトの前提、完了条件、言語など）と、`docs/README.md`
  への道。詳細は置かず、文書へのリンクにする。
- **関係**：節はターゲットの契約で決まる。
- **当てはめた原則**：[段階的な開示](../reference/principles.md#progressive-disclosure)：常に読み込まれるので前提と行き先
  だけ。[スキーマを先に](../reference/principles.md#schema-first)：節を決めて揃える。

### `docs/README.md`

- **責務**：そのリポジトリの文書の唯一の一覧。
- **置くもの / 置かないもの**：文書ごとに、種類と 1 行の説明。中身は置かない。
- **当てはめた原則**：[SSOT](../reference/principles.md#ssot)、[段階的な開示](../reference/principles.md#progressive-disclosure)。

### `docs/overview.md`

- **責務**：そのリポジトリの全体の解説。狙い、全体の構成と部分の間の関係、目指す姿。
- **置くもの / 置かないもの**：部分の細部は置かず、`architecture/` の各文書へリンクする。目指す姿は今の方針として
  書き、そこへの道筋（何をどの順でやるか）は Issue に置く。
- **当てはめた原則**：[関心の分離](../reference/principles.md#soc)：全体と部分を分ける。
  [判断の記録](../reference/principles.md#adr)：道筋と理由はここに置かない。

### `docs/architecture/`

- **責務**：今の姿の解説。部分ごとに、その構成と、構成要素の説明。
- **置くもの / 置かないもの**：文書ごとに一つの部分（例：セッション、コードの層、devcontainer）。冒頭にその部分の
  構成と関係、続けて構成要素の説明を[形式](../reference/formats.md#component)で書く。選ばなかった案は置かず ADR に。
- **関係**：構成要素の説明から原則と ADR へリンクする。
- **当てはめた原則**：[文書の種類](../reference/principles.md#diataxis)の解説。
  [判断の記録](../reference/principles.md#adr)：ADR の連鎖をたどらなくても今の姿が分かるようにする。

### `docs/guides/`

- **責務**：手順。目的を果たすために何をどの順で行うか（準備、追加、移行、復旧）。
- **置くもの / 置かないもの**：手順と、その場で要る最小の説明。仕組みの説明は `architecture/` へリンクする。
  エージェントが使う手順は、スキルにしてもよい。
- **当てはめた原則**：[文書の種類](../reference/principles.md#diataxis)の手順。

### `docs/reference/`

- **責務**：引くための事実。契約、設定、コマンド、形式、用語、決まり、原則の一覧。
- **置くもの / 置かないもの**：一覧や表。手順と理由は置かない。設定の既定値は、例の設定ファイルを正にする。
- **当てはめた原則**：[文書の種類](../reference/principles.md#diataxis)の参照。
  [スキーマを先に](../reference/principles.md#schema-first)：契約と形式はここに先に書く。

### `docs/adr/`

- **責務**：判断の記録。
- **置くもの / 置かないもの**：[形式](../reference/formats.md#adr)に沿う。今の姿のまとめは置かない。
- **当てはめた原則**：[判断の記録](../reference/principles.md#adr)、[メタデータ](../reference/principles.md#metadata)：
  状態と参照を frontmatter に持たせて検査する。

### 文書の外

コード中のコメント、コミットメッセージ、PR に何を書くかは共通のルール（rules プラグイン）にある。道筋と予定は
Issue に置き、文書には書かない（[作業の進め方](../reference/workflow.md)）。

チュートリアルは置かない。使う人が限られ、`guides/` の手順で足りる。

## 決まりの強さ

決まりは、強いものから次の 4 つで守らせ、できるだけ強い方に寄せる（[仕組みで守る](../reference/principles.md#mechanisms)）。

| 強さ | 仕組み | 中身 |
| --- | --- | --- |
| 検査 | `make check-docs`（`tools/check-docs.py`） | リンクと ADR の形（[形式](../reference/formats.md)の各節） |
| 常に読み込む決まり | rules プラグイン | 一つひとつの文の書き方 |
| 必要なときに読む指針 | この文書、[形式](../reference/formats.md)、[文書を書く](../guides/write-docs.md)、review プラグインの観点 | 分け方、置き場所、形 |
| 判断の理由 | ADR | なぜそう決めたか |
