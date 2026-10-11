# 全体像

## 狙い

プロダクトの開発、進行の管理、調べものや研究、壁打ち、日々の雑務など、対象や役割の違うエージェントを簡単に
増やし、安全に動かせる基盤を作る。エージェントは Claude Code で、ターゲット（作業する環境）ごとに隔離した
コンテナで動き、Slack などから依頼を受ける。人は依頼を出し、エージェントが届けた PR を確かめる。

## 考え方

### 何をどこが持つか

| 考え方 | 内容 | 判断 |
| --- | --- | --- |
| リポジトリが宣言する | エージェントの実行環境（devcontainer）、ロールの指示、拡張、規約（CLAUDE.md）は、ターゲットのリポジトリが宣言する。control-plane はそれを読んで動かすだけで、汎用のエージェント像を押し付けない。人間の開発環境とエージェントの実行環境は同じもの | [app 0001](app/adr/0001-repository-centric-agents.md) |
| エージェントはターゲット × ロール | エージェントは「どのターゲットで、どのロールとして働くか」の組で、Slack では Bot 1つに当たる。ロールの振る舞いは platform で共有し、ターゲット固有の事実はターゲットに置く | [app 0002](app/adr/0002-agent-is-target-times-role.md)、[target 0003](target/adr/0003-roles-as-claude-code-agents.md) |
| 共通の能力は platform に | ターゲットをまたいで使うもの（ロール、共通のルール、プラグイン、スキル、hook、コマンド）は platform に置き、ターゲットはそれを宣言して使う | [platform 0001](../platform/docs/adr/0001-platform-as-plugin-marketplace.md) |
| control-plane は場を用意し、進め方はエージェントに任せる | control-plane は環境、認証情報、依頼を届けることと、ターンを始めることを受け持つ。ターンをいつ終えるか、文脈（スレッドやリンク先）をどこまで読むかはエージェントが決める | [app 0017](app/adr/0017-long-lived-session-process.md)、[app 0020](app/adr/0020-slack-requests-by-reference.md) |
| 知識は届く先で区切る | 全体の狙い、設計、ドメインはこのリポジトリに置く。エージェントに要る部分（ハーネスの契約、用語集）は、どのエージェントにも届く platform が持つ。区切りの間は契約でつなぐ | [platform 0005](../platform/docs/adr/0005-documents-by-distribution-context.md) |

### 安全

| 考え方 | 内容 | 判断 |
| --- | --- | --- |
| 隔離と認証情報で守る | 承認の手順や許可するツールの一覧ではなく、コンテナの隔離と、ロールに合わせて絞った認証情報で守る。認証情報は短命にし、control-plane が更新する。コンテナの中では何をしてもよい | [app 0003](app/adr/0003-isolation-as-the-guardrail.md)、[app 0018](app/adr/0018-refreshable-credentials.md) |
| Docker を操作できるのは control-plane だけ | エージェントには Docker ソケットを渡さない。control-plane も、使う API だけを通すプロキシ越しに操作する | [app 0004](app/adr/0004-docker-access-only-via-control-plane.md)、[app 0015](app/adr/0015-docker-api-allowlist-proxy.md) |
| 最後の防衛線は、保護された既定のブランチに入る前の PR | 人は必ず PR を確かめてからマージする。それより前の作業はエージェントに任せ、PR に現れない影響（持ち出し、外部への書き込み）は隔離と認証情報で抑える | [app 0025](app/adr/0025-pr-review-as-the-last-line.md) |

### 作り方

| 考え方 | 内容 | 判断 |
| --- | --- | --- |
| 標準を先に | 拡張は Claude Code の標準機能（プラグイン、スキル、hook）で作り、外部とは公式の API やプロトコルでつなぐ。コンテナの外の準備が要るものだけ、control-plane が解釈する独自の宣言にする | [app 0005](app/adr/0005-extensions-standard-first.md)、[target 0004](target/adr/0004-plugins-declared-in-settings.md) |
| 外部の認証は機能で割り当てる | 拡張は「Slack に投稿できる」のような機能を求め、どの方式（トークン、MCP、webhook など）で渡すかは環境ごとに control-plane が決める。拡張は、渡された機能の組み合わせに合わせて動く | [app 0006](app/adr/0006-external-auth-by-capability.md) |
| プロンプトより仕組み | 守らせたいことは、指示で頼むより、hook、コマンド、検査などの仕組みで確かめる | [app 0021](app/adr/0021-credit-requesters-as-co-authors.md)、[platform 0002](../platform/docs/adr/0002-pr-body-tracks-head.md)、[target 0005](target/adr/0005-ci-runs-in-devcontainer-image.md) |
| ドキュメントは最小、単一、現状 | コードで分かることは書かない。同じ事実は一か所に置き、今の姿だけを書く。理由は ADR に、経緯はコミットと PR に置く（[共通のルール](../platform/plugins/rules/rules/comments-and-docs.md)） | — |
| 必要になってから決める | 規約や共通化は、前もって一度に作らず、使って必要になったものから一つずつ決める。共通部分は、まずコピーして始め、実際に共通だったものだけを切り出す | — |
| 手元から、持ち出せる形で | 手元の Docker で動かすことを先に固める。実行、認証、保存先は差し替えられるようにし、特定のクラウドに依存しない | [app 0007](app/adr/0007-pluggable-claude-auth.md)、[app 0008](app/adr/0008-local-first-portable.md) |

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
  Claude Code（ロール ＝ platform のプラグイン、規約 ＝ ターゲットの CLAUDE.md）
            │
            ▼
GitHub（ブランチ、PR）/ Slack（スレッドへの返信）
```

このリポジトリには、性質の違う3つが同居している。

| 部分 | 中身 | 説明 |
| --- | --- | --- |
| control-plane | `cmd/`、`internal/`、`compose.yaml` | [app](app/)、[codebase](codebase/README.md) |
| platform | `platform/`（ロール、共通のルール、共通のプラグイン） | [platform](../platform/docs/README.md) |
| ターゲットとしての環境 | `.devcontainer/`、`.claude/`、`CLAUDE.md` | [target](target/README.md) |

## 目指す姿

- 上の3つを、それぞれのリポジトリ（agent-control-plane、agent-platform、agent-target-template）に分ける。
  ターゲットはテンプレートから作り、platform のロール、共通のルール、プラグインと、ベースイメージを使う。
- ターゲットは、エージェントが起動する環境（ハーネス：イメージ、設定、プラグイン、規約）と、作業するリポジトリ
  （0 個以上）の組になる。コードを持たない雑務や壁打ちは、作業するリポジトリが 0 個のターゲットで受ける。
- エージェントはロールで役割を分け（developer、guide、reviewer、pm など）、必要なら Bot 同士で依頼し合う。
- Slack からでも手元の CLI からでも、同じセッションに依頼し、途中経過を見られる。

そこへの道筋（何をどの順でやるか）は、ピン留めした
[ロードマップ](https://github.com/Yusaku-KASAI/agent-control-plane/issues/42)の Issue にある。
