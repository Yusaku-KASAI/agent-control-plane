# 全体像

## 狙い

プロダクトの開発、進行の管理、調べものや研究、壁打ち、日々の雑務など、対象や役割の違うエージェントを簡単に
増やし、安全に動かせる基盤を作る。エージェントは Claude Code で、ターゲット（作業する環境）ごとに隔離した
コンテナで動き、Slack などから依頼を受ける。人は依頼を出し、エージェントが届けた PR を確かめる。

## 考え方

### 何をどこが持つか

| 考え方 | 内容 | 判断 |
| --- | --- | --- |
| リポジトリが宣言する | エージェントの実行環境（devcontainer）、有効にするプラグイン、CLAUDE.md は、ターゲットのリポジトリが宣言する。control-plane はそれを読んで動かすだけで、汎用のエージェント像を押し付けない。人間の開発環境とエージェントの実行環境は同じもの | [app 0001](adr/app/0001-repository-centric-agents.md) |
| エージェントはターゲット × ロール | エージェントは「どのターゲットで、どのロールとして働くか」の組で、Slack では Bot 1つに当たる。ロールの定義（振る舞い）は platform の roles プラグインで共有し、ターゲット固有の事実はターゲットの CLAUDE.md に置く。ターゲットは、同じ名前のロールを `.claude/agents/<ロール>.md` に定義して上書きできる（[プラグインとロール](../template/docs/reference/contract.md#plugins "案内")） | [app 0002](adr/app/0002-agent-is-target-times-role.md)、[target-template 0003](../template/docs/adr/0003-roles-as-claude-code-agents.md) |
| 共通の能力は platform に | ターゲットをまたいで使う能力（ロール、スキル、hook、コマンド）は platform のプラグインに置く。共通のルール（principles、conventions）は platform に属さず、同じマーケットプレイスでプラグインとして配る。ターゲットは、使うプラグインを `.claude/settings.json` で有効にする | [platform 0001](../platform/docs/adr/0001-platform-as-plugin-marketplace.md) |
| control-plane は場を用意し、進め方はエージェントに任せる | control-plane は環境、認証情報、依頼を届けることと、ターンを始めることを受け持つ。ターンをいつ終えるか、文脈（スレッドやリンク先）をどこまで読むかはエージェントが決める | [app 0017](adr/app/0017-long-lived-session-process.md)、[app 0020](adr/app/0020-slack-requests-by-reference.md) |
| 知識は届く先で区切る | 知識は、使う人と持ち主ごとの区切りに分け、区切りの間は共有の核（principles、conventions）と契約だけでつなぐ（[地図](#map)） | [conventions 0002](../platform/plugins/conventions/docs/adr/0002-documents-by-distribution-context.md)、[0003](../platform/plugins/conventions/docs/adr/0003-type-first-docs-and-platform-kernel.md)、[0005](../platform/plugins/conventions/docs/adr/0005-principles-and-conventions-plugins.md) |

### 安全

| 考え方 | 内容 | 判断 |
| --- | --- | --- |
| 隔離と認証情報で守る | 承認の手順や許可するツールの一覧ではなく、コンテナの隔離と、ロールに合わせて絞った認証情報で守る。認証情報は短命にし、control-plane が更新する。コンテナの中では何をしてもよい。[最小権限](../platform/plugins/principles/docs/reference/principles.md#least-privilege)：渡した認証情報でできることは、すべて行われうる前提で絞る。依頼やエージェントが読む内容に紛れた指示でエージェントが動かされても、できることはこの範囲に収まる（[外から入る値を信用しない](../platform/plugins/principles/docs/reference/principles.md#untrusted-input)） | [app 0003](adr/app/0003-isolation-as-the-guardrail.md)、[app 0018](adr/app/0018-refreshable-credentials.md) |
| Docker を操作できるのは control-plane だけ | エージェントには Docker ソケットを渡さない。control-plane も、使う API だけを通すプロキシ越しに操作する（[最小権限](../platform/plugins/principles/docs/reference/principles.md#least-privilege)、[拒否を既定にする](../platform/plugins/principles/docs/reference/principles.md#fail-safe-defaults)）。control-plane の Docker 操作は、エージェントの入力から直接組み立てない（[外から入る値を信用しない](../platform/plugins/principles/docs/reference/principles.md#untrusted-input)） | [app 0004](adr/app/0004-docker-access-only-via-control-plane.md)、[app 0015](adr/app/0015-docker-api-allowlist-proxy.md) |
| 最後の防衛線は、保護された既定のブランチに入る前の PR | 人は必ず PR を確かめてからマージする。それより前の作業はエージェントに任せ、PR に現れない影響（持ち出し、外部への書き込み）は隔離と認証情報で抑える。[多層の防御](../platform/plugins/principles/docs/reference/principles.md#defense-in-depth)：既定のブランチへの変更は、ルールセット（PR 経由に限る）と人の確認を重ねて止める。PR を作るエージェントの認証情報ではマージもできるので、人の確認は仕組みではなく手順で守る。持ち出しを止める層はなく、持ち出されうるものを隔離（マウントしたものだけ）と認証情報の範囲と期限で限る。外への通信は絞っていない（[target-template 0008](../template/docs/adr/0008-no-in-container-firewall.md)） | [app 0025](adr/app/0025-pr-review-as-the-last-line.md) |

### 作り方

| 考え方 | 内容 | 判断 |
| --- | --- | --- |
| 標準を先に | 拡張は Claude Code の標準機能（プラグイン、スキル、hook）で作り、外部とは公式の API やプロトコルでつなぐ。コンテナの外の準備が要るものだけ、control-plane が解釈する独自の宣言にする | [app 0005](adr/app/0005-extensions-standard-first.md)、[target-template 0004](../template/docs/adr/0004-plugins-declared-in-settings.md) |
| 外部の認証は機能で割り当てる | 拡張は「Slack に投稿できる」のような機能を求め、どの方式（トークン、MCP、webhook など）で渡すかは環境ごとに control-plane が決める。拡張は、渡された機能の組み合わせに合わせて動く | [app 0006](adr/app/0006-external-auth-by-capability.md) |
| プロンプトより仕組み | [仕組みで守る](../platform/plugins/principles/docs/reference/principles.md#mechanisms)。hook、コマンド、検査で確かめる | [app 0021](adr/app/0021-credit-requesters-as-co-authors.md)、[platform 0002](../platform/docs/adr/0002-pr-body-tracks-head.md)、[target-template 0005](../template/docs/adr/0005-ci-runs-in-devcontainer-image.md) |
| 文書は種類で分け、一か所に今だけを書く | 文書は、種類（解説、参照、手順、判断）で分け、同じ事実を一か所にだけ、今の姿だけを書く。拠り所の原則とパターンは principles プラグインに、文書の分け方と書き方の決まりは conventions プラグインの[文書の体系](../platform/plugins/conventions/docs/architecture/documentation.md "案内")にある | [conventions 0002](../platform/plugins/conventions/docs/adr/0002-documents-by-distribution-context.md)、[0003](../platform/plugins/conventions/docs/adr/0003-type-first-docs-and-platform-kernel.md)、[0004](../platform/plugins/conventions/docs/adr/0004-principles-and-patterns.md) |
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

<a id="map"></a>

### 地図

このプロジェクト群の知識を、入れ子の項目に分ける。分け方は conventions の
[項目の階層と関係](../platform/plugins/conventions/docs/architecture/documentation.md#map "案内")に、表の書き方と検査は
[形式](../platform/plugins/conventions/docs/reference/formats.md#map "案内")にある。ここは最上位の地図で、
各項目の中の地図はその項目の文書にある。各ターゲットを除く項目は、このリポジトリに同居する。

<a id="map-top"></a>

| 項目 | 親 | 置き場所 | 前提にする | 持つもの | 届く先 |
| --- | --- | --- | --- | --- | --- |
| principles | | `../platform/plugins/principles/` | | どこでも通用する原則とパターン | どこでも（プラグインとして入れる） |
| conventions | | `../platform/plugins/conventions/` | principles | このプロジェクト群に共通の決まり（文書の分け方と形式、変更と課題の扱い方、書き方、設計の観点） | このプロジェクト群の各リポジトリと、そこで動くエージェント |
| 契約 | | | principles, conventions | 区切りの間の約束 | 契約の両側 |
| エージェントの用語集 | 契約 | `../platform/docs/reference/glossary.md` | | エージェントとハーネスの言葉 | |
| ハーネスの契約 | 契約 | `../platform/docs/reference/harness.md` | エージェントの用語集, ターゲットの契約 | control-plane がエージェントに渡すもの | |
| ターゲットの契約 | 契約 | `../template/docs/reference/contract.md` | エージェントの用語集 | ターゲットが用意するもの | |
| platform | | `../platform/` | principles, conventions, 契約 | エージェントの能力（ロール、スキル、hook） | すべてのエージェント |
| target-template | | `../template/` | principles, conventions, 契約, platform | ターゲットに共通の環境のファイルと、その説明 | すべてのターゲット（テンプレートから作る） |
| control-plane | | `../README.md`, `../CLAUDE.md`, `./` | principles, conventions, 契約, platform, target-template | 全体の設計、アプリの仕組み、Go の書き方、運用の手順 | control-plane の開発者 |
| 各ターゲット | | | principles, conventions, 契約, platform, target-template | プロダクトの前提、そのリポジトリの仕組みと判断 | そのターゲットのエージェント |

- 契約と各ターゲットは、置き場所を持たない。契約は中の項目の集まりで、各ターゲットはこのリポジトリの外にある。
- principles、conventions、ハーネスの契約、エージェントの用語集は、ほかのターゲットのエージェントに届くよう
  マーケットプレイス（`platform/`）の中に置くが、platform には属さない。ターゲットの契約は `template/` の中に置くが、
  target-template には属さない。

<a id="map-control-plane"></a>

#### control-plane の中

子に属さない文書（この文書、索引、[Issue のラベル](reference/issues.md "案内")など）は、control-plane の全体。

| 項目 | 親 | 置き場所 | 前提にする |
| --- | --- | --- | --- |
| コードベース | control-plane | `architecture/codebase/`, `reference/coding.md`, `adr/codebase/` | |
| アプリ | control-plane | `architecture/app/`, `reference/usage.md`, `guides/operations.md`, `adr/app/` | コードベース |

## 目指す姿

- このリポジトリに同居する control-plane、platform、target-template を、それぞれのリポジトリ（agent-control-plane、
  agent-platform、agent-target-template）に分ける。ターゲットはテンプレートから作り、platform のプラグイン、
  同じマーケットプレイスで配る共通のルール（principles、conventions）、ベースイメージを使う。
- ターゲットは、エージェントが起動する環境（イメージ、設定、プラグインの宣言、CLAUDE.md）と、作業するリポジトリ
  （0 個以上）の組になる。このうちイメージとプラグインはハーネス（[用語集](../platform/docs/reference/glossary.md "案内")）に入り、
  作業するリポジトリは入らない。コードを持たない雑務や壁打ちは、作業するリポジトリが 0 個のターゲットで受ける。
- エージェントはロールで役割を分け（developer、guide、reviewer、pm など）、必要なら Bot 同士で依頼し合う。
- Slack からでも手元の CLI からでも、同じセッションに依頼し、途中経過を見られる。

そこへの道筋（何をどの順でやるか）は、ピン留めした
[ロードマップ](https://github.com/Yusaku-KASAI/agent-control-plane/issues/42)の Issue にある。
