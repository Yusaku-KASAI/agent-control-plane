# 全体像

## 狙い

プロダクトの開発、進行の管理、調べものや研究、壁打ち、日々の雑務など、対象や役割の違うエージェントを簡単に
増やし、安全に動かせる基盤を作る。エージェントは Claude Code で、ターゲット（作業する環境）ごとに隔離した
コンテナで動き、Slack などから依頼を受ける。人は依頼を出し、エージェントが届けた PR を確かめる。

## 考え方

### 何をどこが持つか

| 考え方 | 内容 | 判断 |
| --- | --- | --- |
| リポジトリが宣言する | エージェントの実行環境（devcontainer）、ロールの指示、拡張、CLAUDE.md は、ターゲットのリポジトリが宣言する。control-plane はそれを読んで動かすだけで、汎用のエージェント像を押し付けない。人間の開発環境とエージェントの実行環境は同じもの | [app 0001](adr/app/0001-repository-centric-agents.md) |
| エージェントはターゲット × ロール | エージェントは「どのターゲットで、どのロールとして働くか」の組で、Slack では Bot 1つに当たる。ロールの振る舞いは platform で共有し、ターゲット固有の事実はターゲットに置く | [app 0002](adr/app/0002-agent-is-target-times-role.md)、[target 0003](../template/docs/adr/0003-roles-as-claude-code-agents.md) |
| 共通の能力は platform に | ターゲットをまたいで使うもの（ロール、共通のルール、プラグイン、スキル、hook、コマンド）は platform に置き、ターゲットはそれを宣言して使う | [platform 0001](../platform/docs/adr/0001-platform-as-plugin-marketplace.md) |
| control-plane は場を用意し、進め方はエージェントに任せる | control-plane は環境、認証情報、依頼を届けることと、ターンを始めることを受け持つ。ターンをいつ終えるか、文脈（スレッドやリンク先）をどこまで読むかはエージェントが決める | [app 0017](adr/app/0017-long-lived-session-process.md)、[app 0020](adr/app/0020-slack-requests-by-reference.md) |
| 知識は届く先で区切る | 知識は、使う人と持ち主ごとの区切りに分け、区切りの間は共有の核（principles、conventions）と契約だけでつなぐ（[区切り](#contexts)） | [conventions 0002](../platform/plugins/conventions/docs/adr/0002-documents-by-distribution-context.md)、[0003](../platform/plugins/conventions/docs/adr/0003-type-first-docs-and-platform-kernel.md)、[0005](../platform/plugins/conventions/docs/adr/0005-principles-and-conventions-plugins.md) |

### 安全

| 考え方 | 内容 | 判断 |
| --- | --- | --- |
| 隔離と認証情報で守る | 承認の手順や許可するツールの一覧ではなく、コンテナの隔離と、ロールに合わせて絞った認証情報で守る。認証情報は短命にし、control-plane が更新する。コンテナの中では何をしてもよい | [app 0003](adr/app/0003-isolation-as-the-guardrail.md)、[app 0018](adr/app/0018-refreshable-credentials.md) |
| Docker を操作できるのは control-plane だけ | エージェントには Docker ソケットを渡さない。control-plane も、使う API だけを通すプロキシ越しに操作する | [app 0004](adr/app/0004-docker-access-only-via-control-plane.md)、[app 0015](adr/app/0015-docker-api-allowlist-proxy.md) |
| 最後の防衛線は、保護された既定のブランチに入る前の PR | 人は必ず PR を確かめてからマージする。それより前の作業はエージェントに任せ、PR に現れない影響（持ち出し、外部への書き込み）は隔離と認証情報で抑える | [app 0025](adr/app/0025-pr-review-as-the-last-line.md) |

### 作り方

| 考え方 | 内容 | 判断 |
| --- | --- | --- |
| 標準を先に | 拡張は Claude Code の標準機能（プラグイン、スキル、hook）で作り、外部とは公式の API やプロトコルでつなぐ。コンテナの外の準備が要るものだけ、control-plane が解釈する独自の宣言にする | [app 0005](adr/app/0005-extensions-standard-first.md)、[target 0004](../template/docs/adr/0004-plugins-declared-in-settings.md) |
| 外部の認証は機能で割り当てる | 拡張は「Slack に投稿できる」のような機能を求め、どの方式（トークン、MCP、webhook など）で渡すかは環境ごとに control-plane が決める。拡張は、渡された機能の組み合わせに合わせて動く | [app 0006](adr/app/0006-external-auth-by-capability.md) |
| プロンプトより仕組み | [仕組みで守る](../platform/plugins/principles/docs/reference/principles.md#mechanisms)。hook、コマンド、検査で確かめる | [app 0021](adr/app/0021-credit-requesters-as-co-authors.md)、[platform 0002](../platform/docs/adr/0002-pr-body-tracks-head.md)、[target 0005](../template/docs/adr/0005-ci-runs-in-devcontainer-image.md) |
| 文書は種類で分け、一か所に今だけを書く | 原則とパターンは principles、文書の分け方と書き方は conventions の[文書の体系](../platform/plugins/conventions/docs/architecture/documentation.md)にある | [conventions 0002](../platform/plugins/conventions/docs/adr/0002-documents-by-distribution-context.md)、[0003](../platform/plugins/conventions/docs/adr/0003-type-first-docs-and-platform-kernel.md)、[0004](../platform/plugins/conventions/docs/adr/0004-principles-and-patterns.md) |
| 必要になってから決める | [使ってから共通にする](../platform/plugins/principles/docs/reference/patterns.md#extract-after-use)。共通部分はまずコピーして始め、実際に共通だったものだけを切り出す | — |
| 手元から、持ち出せる形で | 手元の Docker で動かすことを先に固める。実行、認証、保存先は差し替えられるようにし、特定のクラウドに依存しない | [app 0007](adr/app/0007-pluggable-claude-auth.md)、[app 0008](adr/app/0008-local-first-portable.md) |

## 構成

```
依頼者 ── Slack（エージェントごとの Bot）/ acp run（HTTP API）
            │
            ▼
control-plane（acp serve）──── Docker のプロキシ（許可した API だけ）
  セッションの管理、作業領域の clone、認証情報の発行と更新
            │
            ▼
エージェントのコンテナ（セッションごと）
  ターゲットのイメージ ＋ セッションの作業領域 ＋ 短命の認証情報
  Claude Code（ロール ＝ platform のプラグイン、ターゲットの CLAUDE.md）
            │
            ▼
GitHub（ブランチ、PR）/ Slack（スレッドへの返信）
```

<a id="contexts"></a>

### 地図

このプロジェクト群の知識を、入れ子の項目に分ける。決まりは conventions の
[項目の階層と関係](../platform/plugins/conventions/docs/architecture/documentation.md#map)。切り出すまでは、各ターゲットを除く
項目がこのリポジトリに同居する。

| 項目 | 持つもの | 届く先 |
| --- | --- | --- |
| principles | どこでも通用する原則とパターン | どこでも（プラグインとして入れる） |
| conventions | このプロジェクト群の決まり（文書の体系と形式、作業の進め方、書き方、設計の観点） | このプロジェクト群のすべてのリポジトリとエージェント |
| 契約 | 区切りの間の約束：エージェントとハーネスの用語集、ハーネスの契約（control-plane がエージェントに渡すもの）、ターゲットの契約（ターゲットが用意するもの） | 契約の両側 |
| platform | エージェントの能力（ロール、スキル、hook） | すべてのエージェント |
| target-template | ターゲットに共通の環境のファイルと、その説明 | すべてのターゲット（テンプレートから作る） |
| control-plane | 全体の設計、アプリの仕組み、Go の書き方、運用の手順 | control-plane の開発者 |
| 各ターゲット | プロダクトの前提、そのリポジトリの仕組みと判断 | そのターゲットのエージェント |

- principles と conventions は、ほかのすべての項目が前提にしてよい共有の核。
- 契約は、作る側（control-plane）にも使う側（platform のプラグイン、エージェント、各ターゲット）にも属さない。両側が
  契約を前提にし、互いの中身は前提にしない（依存関係の逆転）。契約が多くから前提にされるので、変わりにくく小さく保つ。
- 置き場所と関係は分けて考える。principles、conventions、契約の一部は、ほかのターゲットのエージェントに届くよう
  マーケットプレイス（`platform/`）の中に置くが、関係の上では platform に属さない。

<a id="map-table"></a>

#### 項目と前提の向き

項目ごとの親、置き場所、前提にする兄弟（同じ親の下の項目）。`make check-docs` がこの表を読み、文書のリンクが向きに
反していないかと、循環がないかを確かめる（[形式](../platform/plugins/conventions/docs/reference/formats.md#map)）。
設計構造行列は `python3 tools/check-docs.py --map` で出す。

| 項目 | 親 | 置き場所 | 前提にする |
| --- | --- | --- | --- |
| principles | | `platform/plugins/principles/` | |
| 原則 | principles | `platform/plugins/principles/docs/reference/principles.md` | |
| パターン | principles | `platform/plugins/principles/docs/reference/patterns.md` | 原則 |
| conventions | | `platform/plugins/conventions/` | principles |
| 契約 | | | principles, conventions |
| 用語集 | 契約 | `platform/docs/reference/glossary.md` | |
| ハーネスの契約 | 契約 | `platform/docs/reference/harness.md` | 用語集 |
| ターゲットの契約 | 契約 | `template/docs/reference/contract.md` | 用語集, ハーネスの契約 |
| platform | | `platform/` | principles, conventions, 契約 |
| harness | platform | `platform/plugins/harness/` | |
| github | platform | `platform/plugins/github/` | |
| slack | platform | `platform/plugins/slack/` | |
| roles | platform | `platform/plugins/roles/` | harness, github, slack |
| bell | platform | `platform/plugins/bell/` | |
| target-template | | `template/` | principles, conventions, 契約, platform |
| devcontainer | target-template | `template/docs/architecture/devcontainer.md`, `template/docs/guides/devcontainer.md` | |
| CI | target-template | `template/docs/architecture/ci.md` | devcontainer |
| control-plane | | `README.md`, `CLAUDE.md`, `docs/` | principles, conventions, 契約, platform, target-template |
| コード | control-plane | `docs/architecture/codebase/` | |
| アプリ | control-plane | `docs/architecture/app/` | コード |
| 各ターゲット | | | principles, conventions, 契約, platform, target-template |

## 目指す姿

- このリポジトリに同居する control-plane、platform、target-template を、それぞれのリポジトリ（agent-control-plane、
  agent-platform、agent-target-template）に分ける。ターゲットはテンプレートから作り、platform のロール、共通のルール、
  プラグインと、ベースイメージを使う。
- ターゲットは、エージェントが起動する環境（イメージ、設定、プラグイン、CLAUDE.md。control-plane と合わせて
  [ハーネス](../platform/docs/reference/glossary.md)になる）と、作業するリポジトリ（0 個以上）の組になる。コードを
  持たない雑務や壁打ちは、作業するリポジトリが 0 個のターゲットで受ける。
- エージェントはロールで役割を分け（developer、guide、reviewer、pm など）、必要なら Bot 同士で依頼し合う。
- Slack からでも手元の CLI からでも、同じセッションに依頼し、途中経過を見られる。

そこへの道筋（何をどの順でやるか）は、ピン留めした
[ロードマップ](https://github.com/Yusaku-KASAI/agent-control-plane/issues/42)の Issue にある。
