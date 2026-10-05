# 全体像

## 狙い

Claude Code のエージェントを、ターゲット（作業する環境）ごとに隔離したコンテナで動かし、Slack などから依頼
できるようにする。ターゲットを増やす手間を小さくし、人が依頼を出すだけで、エージェントが変更を PR として届ける
状態を目指す。

## 考え方

| 考え方 | 内容 | 判断 |
| --- | --- | --- |
| リポジトリが宣言する | エージェントの実行環境（devcontainer）、ロールの指示、拡張、規約（CLAUDE.md）は、ターゲットのリポジトリが宣言する。control-plane はそれを読んで動かすだけで、汎用のエージェント像を押し付けない。人間の開発環境とエージェントの実行環境は同じもの | [app 0001](app/adr/0001-repository-centric-agents.md) |
| エージェントはターゲット × ロール | エージェントは「どのターゲットで、どのロールとして働くか」の組。ロールの振る舞いは platform で共有し、ターゲットの事実はターゲットに置く | [app 0002](app/adr/0002-agent-is-target-times-role.md)、[target 0003](target/adr/0003-roles-as-claude-code-agents.md) |
| 安全は隔離と認証情報で守る | 承認の手順や許可するツールの一覧ではなく、コンテナの隔離と、ロールに合わせて絞った短命の認証情報で守る。コンテナの中では何をしてもよく、Docker を操作できるのは control-plane だけ | [app 0003](app/adr/0003-isolation-as-the-guardrail.md)、[app 0004](app/adr/0004-docker-access-only-via-control-plane.md) |
| 拡張は標準を先に | コンテナの中で完結する拡張は Claude Code の標準（プラグイン、スキル、hook）で作る。コンテナの外の準備が要るものだけ、control-plane が解釈する独自の宣言にする | [app 0005](app/adr/0005-extensions-standard-first.md)、[platform 0001](platform/adr/0001-platform-as-plugin-marketplace.md) |
| 外部の認証は機能で割り当てる | 拡張は「Slack に投稿できる」のような機能を求め、どの方式（トークン、MCP など）で渡すかは環境ごとに control-plane が決める | [app 0006](app/adr/0006-external-auth-by-capability.md) |
| 手元から、持ち出せる形で | 手元の Docker で動かすことを先に固める。実行、認証、保存先は差し替えられるようにし、特定のクラウドに依存しない | [app 0008](app/adr/0008-local-first-portable.md) |

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
| platform | `platform/`（ロールと共通のプラグイン） | [platform](platform/README.md) |
| ターゲットとしての環境 | `.devcontainer/`、`.claude/`、`CLAUDE.md` | [target](target/README.md) |

## 目指す姿

- 上の3つを、それぞれのリポジトリ（agent-control-plane、agent-platform、agent-target-template）に分ける。
  ターゲットはテンプレートから作り、platform のプラグインとベースイメージを使う。
- ターゲットは、エージェントが起動する環境（ハーネス：イメージ、設定、プラグイン、規約）と、作業するリポジトリ
  （0 個以上）の組になる。コードを持たない雑務や壁打ちは、作業するリポジトリが 0 個のターゲットで受ける。
- エージェントはロールで役割を分け（developer、guide、reviewer など）、必要なら Bot 同士で依頼し合う。

そこへの道筋（何をどの順でやるか）は、ピン留めした
[ロードマップ](https://github.com/Yusaku-KASAI/agent-control-plane/issues/42)の Issue にある。
