# ターゲット

このリポジトリを、エージェントが作業するターゲットとして成り立たせる環境（devcontainer、Claude Code の設定、CI、
作業の進め方）。target-template の元になる。

## devcontainer

Anthropic の [Claude Code devcontainer](https://github.com/anthropics/claude-code/tree/main/.devcontainer) を元にした、
人の開発環境とエージェントの実行環境を兼ねるコンテナ（`.devcontainer/`）。

### 初回セットアップ（ホストで一度だけ）

```bash
docker network create agent-control-plane-net
docker volume create agent-control-plane_claude-code-bashhistory
docker volume create agent-control-plane_claude-code-config
# Build node with your UID/GID, so that the container needs no realignment at start.
printf 'HOST_UID=%s\nHOST_GID=%s\n' "$(id -u)" "$(id -g)" > .devcontainer/.env
```

### 方針

- コンテナの振る舞いは Dockerfile、compose.yaml、entrypoint で決める。devcontainer.json は VS Code 側の
  配線だけを持つ（[ADR 0001](adr/0001-devcontainer-independent-of-vscode.md)）。
- `base` ステージには、リポジトリを問わずエージェントが使うもの（Node、Python と uv、シェルツール、Claude Code など）
  だけを入れ、リポジトリ名も含めない。リポジトリ固有のツールチェーンは後段のステージに置き、リポジトリの場所は
  compose.yaml の `working_dir` で決める。
- Claude Code とツールの版は Dockerfile の `ARG` で固定する。上げるときは PR で変え、CI でイメージのビルドと
  `make check` を確かめる。Claude Code はネイティブのインストーラーで入れ、自分で更新しないよう
  `DISABLE_UPDATES=1` にする。
- `node` には sudo を与えない。apt のパッケージ一覧はイメージに残し、人が `docker exec -u root` で入って一時的に
  インストールできるようにする（[ADR 0002](adr/0002-no-sudo-for-agent-user.md)）。コンテナの中にファイアウォールは
  置かない（[ADR 0008](adr/0008-no-in-container-firewall.md)）。

### 構成

- イメージは `base` → `go`（このリポジトリのツールチェーン）の 2 段で、`agent-control-plane-devcontainer` という
  名前でビルドされる。control-plane はエージェントも同じイメージで動かす（`acp.yaml` の `target.image`）。
  Go のツールは base と切り離した `go-tools` ステージでビルドし、バイナリだけを `go` にコピーする。base を変えても
  ツールを作り直さない。
- ビルドの文脈は `.devcontainer/Dockerfile.dockerignore` で決める（ルートの `.dockerignore` は control-plane の
  イメージ用）。
- `..` を `/workspace/agent-control-plane` に読み書き可、`../..` を `/workspace` に**読み取り専用**で
  マウントする。隣に置いたリポジトリは参照できるが書き込めない。
- `node` の UID と GID は、ビルドのときにホストの値（`.devcontainer/.env`）に合わせる。ずれていたとき
  （CI など）だけ、起動時に entrypoint がリポジトリの所有者に合わせ直し、所有者の違うボリュームを直す。

### Claude Code の設定

`.claude/settings.json` は人が対話で使うときの設定。エージェントは `--dangerously-skip-permissions` で動くので、
`permissions` は効かない。コンテナの隔離で守る方針なので、対話でも権限の確認を省いて（bypass）使ってよい。
`permissions.allow` の一覧は、組織の制限などで bypass が使えない環境で確認を減らすためにある。

### tmux とターミナルのベル

hook のプロセスは制御端末を持たないため、hook からベルを届ける経路は `tmux list-clients` しかない。
VS Code の既定のターミナルプロファイルが tmux なのはこのため。

### VS Code の設定

エディタ設定は `devcontainer.json` の `customizations.vscode.settings`（コンテナに attach したウィンドウ）と
`.vscode/settings.json`（attach していないウィンドウ）に同じ内容で置く。ベル設定のように、片方だけだと
効いたり効かなかったりするものがあるため。同じであることは `make lint` で確かめる。

## イメージに求めること

control-plane はセッションごとにターゲットのイメージからコンテナを作り、`claude -p` を `stream-json` の
入出力で動かし続けて、依頼を標準入力に送る。そのためにイメージは次を満たす必要がある。

- entrypoint が root で動き、準備を済ませたあと引数のコマンドを `exec` する。標準入力はそのまま渡す。
- エージェントを動かすユーザー `node` と、`claude`、`runuser`、`git`、`gh` が入っている。control-plane は
  `runuser -u node -- claude ...` を実行する（Claude Code は root では権限確認を省略できないため）。
- entrypoint は環境変数 `WORKSPACE_DIR`（リポジトリのマウント先）を解釈する。
- entrypoint 自身の出力は stderr に出す。stdout はエージェントの回答として扱われる。

control-plane はさらに次の環境変数を渡す。

| 変数 | 内容 |
| --- | --- |
| `ACP_SESSION_BRANCH` | セッションの既定の作業ブランチ |
| `ACP_CO_AUTHORS_FILE` | 今のターンの依頼者を 1 行に 1 人（`名前 <メール>`）で書いたファイル |
| `SLACK_BOT_TOKEN_FILE` | エージェントの Slack アプリの Bot トークンを置いたファイル（Slack アプリがある場合） |
| `GIT_AUTHOR_*` / `GIT_COMMITTER_*` | エージェントのコミットの作者（設定されている場合） |
| `GH_CONFIG_DIR` と `GIT_CONFIG_*` | GitHub のトークンを置いた `gh` の設定ディレクトリと、git がそれを `gh` 経由で使う設定（GitHub App がある場合） |

認証情報は `/run/acp/credentials` に読み取り専用でマウントされ、control-plane が失効前に書き換える。

作業領域の `.git/hooks/commit-msg` には control-plane がフックを置き、`ACP_CO_AUTHORS_FILE` の人を
`Co-authored-by` としてコミットに加える。ターゲットが `core.hooksPath` を設定していると、このフックは使われない。

## プラグインとロール

- ターゲットが使うプラグインは `.claude/settings.json` の `extraKnownMarketplaces` と `enabledPlugins` で宣言する
  （[ADR 0004](adr/0004-plugins-declared-in-settings.md)）。control-plane は、`directory` のマーケットプレイスから
  有効なプラグインを `--plugin-dir` で読み込ませる。それ以外のマーケットプレイスのプラグインは Claude Code に任せる。
  - マーケットプレイスの場所は、devcontainer でのパス `/workspace/<リポジトリ>/...` で書く。ターゲット自身の
    中を指すパスは、エージェントのコンテナではセッションの作業領域（作業中のブランチ）を指す。隣のリポジトリを
    指すパスは、control-plane がホストでターゲットの隣にあるそのディレクトリだけを、同じパスに読み取り専用で
    マウントする。ただし、ホストでシンボリックリンクをたどった先がターゲット自身の中なら、作業領域の中として扱う
    （[ADR 0007](adr/0007-marketplaces-beside-the-target.md)）。
- ロールは Claude Code のカスタムエージェントとして定義し、control-plane は `claude -p --agent <ロール名>` で
  起動する（[ADR 0003](adr/0003-roles-as-claude-code-agents.md)）。共有のロールは [platform](../platform/README.md) の
  `roles` プラグインにあり、ターゲットの `.claude/agents/<ロール名>.md` に同名の定義を置くとそちらが優先される。
- ロールの定義には振る舞いだけを書き、ターゲット固有の事実や規約は `CLAUDE.md` に書く。
- 定義が見つからないロールで起動すると、エージェントはエラーで終了する。

## CI

PR と main への push ごとに、devcontainer のイメージの中で `make check` を実行する
（[ADR 0005](adr/0005-ci-runs-in-devcontainer-image.md)）。イメージを書き出さず、Dockerfile の `check` ステージ
（`go` の上でリポジトリをコピーして `make check`）としてビルドする。層は GitHub Actions のキャッシュに残し、
Go のモジュールは go.sum が変わらない限りキャッシュから使う。あわせて control-plane のイメージがビルドできることを
確かめる。

## 作業の進め方

- main への変更は PR 経由で行い、merge commit でマージする。PR のコミットがそのまま main に残る。
- やることは GitHub の Issue で管理する。ドキュメントには現状だけを書くので、予定や残りの作業は置かない。
  分類は木にせず、性質ごとに分けて持つ（[ADR 0006](adr/0006-issues-in-layers.md)）。

  | 表すもの | 置き場所 |
  | --- | --- |
  | やること1件 | Issue |
  | どこを変えるか | ラベル `app`、`codebase`、`target`、`platform`（docs の関心）。複数付けてよい |
  | 何のためか | ラベル `theme:<名前>`。複数付けてよい |
  | テーマの狙い、終えたときの状態、進め方 | ラベル `theme` の Issue。メンバーは本文に並べず、ラベルで決まる |
  | 何が先か | Issue の依存関係（blocked by） |
  | 今の焦点と順番 | ピン留めした「ロードマップ」の Issue。順番を変えるときは本文を書き換える |

  - sub-issues は、大きな作業を手順に分けるときだけ使う。
  - 検討中の設計は Issue に書き、決まったら ADR にする。
  - 気づいているリスクや、壊してはいけない前提は、会話や依頼文だけでなく Issue の本文に書く。作業するエージェントは
    Issue とリポジトリから読み取れることしか知らない。
- PR の作成と更新は platform の github スキル（`prctl`）に従う。PR が Issue を片付けるときは、本文に
  `Closes #<番号>` と書く。
