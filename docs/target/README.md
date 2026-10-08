# ターゲット

このリポジトリを、エージェントが作業するターゲットとして成り立たせる環境（devcontainer、Claude Code の設定、CI、
作業の進め方）。target-template の元になる。

## devcontainer

Anthropic の [Claude Code devcontainer](https://github.com/anthropics/claude-code/tree/main/.devcontainer) を元にした、
人の開発環境とエージェントの実行環境を兼ねるコンテナ（`.devcontainer/`）。

### ホストの準備

VS Code は、コンテナを起動する前に毎回 `.devcontainer/init-host.sh` を実行する（`initializeCommand`）。
VS Code を使わないときは一度だけ手で実行する。このスクリプトは次を行い、`.env` にすでにある値は変えない。

- `.devcontainer/.env`（gitignore の対象）に、リポジトリ名（`REPO_NAME`）、ホストの UID と GID（`HOST_UID` /
  `HOST_GID`）、状態を置くディレクトリ（`DEV_STATE_DIR`、既定は `~/.local/state/devcontainers/<リポジトリ名>`）を書く。
- 状態を置くディレクトリと、control-plane と共有するネットワーク（`agent-control-plane-net`）を作る。

compose.yaml と devcontainer.json は、名前やパスをこれらの値から決めるので、ターゲットが変わっても書き換えない。

### 方針

- コンテナの振る舞いは Dockerfile、compose.yaml、entrypoint で決める。devcontainer.json は VS Code 側の
  配線だけを持つ（[ADR 0001](adr/0001-devcontainer-independent-of-vscode.md)）。
- `base` ステージには、リポジトリを問わずエージェントが使うもの（Node、Python と uv、シェルツール、Claude Code など）
  だけを入れ、リポジトリやホストに依存する値（リポジトリ名、タイムゾーン、UID）を含めない。層は変わりにくいものから
  順に並べる。リポジトリ固有のツールチェーンとホストに依存する値は後段のステージに置く。
- RUN は `pipefail` 付きの bash で動かし、パイプの途中の失敗（ダウンロードの失敗など）でビルドを止める。外から
  取得するものは版を固定する。
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
- `..` を `/workspace/<リポジトリ名>` に読み書き可、`../..` を `/workspace` に**読み取り専用**で
  マウントする。隣に置いたリポジトリは参照できるが書き込めない。
- Claude Code の設定（ログイン、履歴、メモリ）、`gh` の設定、シェルの履歴は、ホストの `DEV_STATE_DIR` の下に置いて
  マウントする。Docker の外にあるので、`docker compose down -v` や prune でも消えない
  （[ADR 0010](adr/0010-devcontainer-state-on-the-host.md)）。Go のキャッシュは消えても困らないので、名前付きボリュームに置く。
- コンテナは root を使わず、数値の UID と GID の `node` で動く（[ADR 0009](adr/0009-containers-run-as-non-root.md)）。
  UID と GID は、ビルドのときにリポジトリの所有者の値（`.devcontainer/.env`）に合わせる。control-plane の
  `.env` の `ACP_UID` / `ACP_GID` も同じ値にする。ボリュームのマウント先はイメージの中に `node` の持ち物として
  作ってあり、新しい名前付きボリュームはその所有者を引き継ぐ。root で作業したいときは `docker exec -u root` で入る。

### Claude Code の設定

`.claude/settings.json` は人が対話で使うときの設定。エージェントは `--dangerously-skip-permissions` で動くので、
`permissions` は効かない。コンテナの隔離で守る方針なので、対話でも権限の確認を省いて（bypass）使ってよい。
`permissions.allow` の一覧は、組織の制限などで bypass が使えない環境で確認を減らすためにある。

### tmux とターミナルのベル

- マウスでの選択（ドラッグ、ダブルクリック、トリプルクリック）は選択するだけで、クリップボードに自動ではコピーしない。
  選択は残り、`y` か `Enter` でコピーする。`q` か `Escape` で選択をやめる。
- Claude Code が対話で止まったとき（ターンの終わり、権限の確認、質問）は、platform の bell プラグインが端末の
  ベルを鳴らす（応答を待つときは2回、ターンの終わりは1回）。hook のプロセスは制御端末を持たないため、ベルは tmux に
  つながった端末に書く。VS Code の既定のターミナルプロファイルが tmux なのはこのため。tmux は、ベルを外の端末に
  渡しつつ、自分でも表示する。

### VS Code の設定

エディタ設定は `devcontainer.json` の `customizations.vscode.settings`（コンテナに attach したウィンドウ）と
`.vscode/settings.json`（attach していないウィンドウ）に同じ内容で置く。ベル設定のように、片方だけだと
効いたり効かなかったりするものがあるため。同じであることは `make check-target` で確かめる。

## イメージに求めること

control-plane はセッションごとにターゲットのイメージからコンテナを作り、`claude -p` を `stream-json` の
入出力で動かし続けて、依頼を標準入力に送る。そのためにイメージは次を満たす必要がある。

- イメージのユーザーが root でなく、作業ディレクトリ（リポジトリのマウント先）に書き込める。control-plane は
  `claude ...` をそのままコマンドとして渡す（Claude Code は root では権限確認を省略できない）。
- `claude`、`git`、`gh` が入っている。
- entrypoint があれば、準備を済ませたあと引数のコマンドを `exec` する。標準入力はそのまま渡し、自身の出力は
  stderr に出す。stdout はエージェントの回答として扱われる。

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
（[ADR 0005](adr/0005-ci-runs-in-devcontainer-image.md)）。あわせて control-plane のイメージがビルドできることを
確かめる（`check-image`）。

- `make check` は関心ごとの検査（`check-go`、`check-platform`、`check-target`）をまとめたもの。
- CI は関心ごとのジョブ（`check-go`、`check-platform`、`check-target`）で、Dockerfile の同じ名前のステージを、
  イメージを書き出さずにビルドする（`check-image` も書き出さない）。各ステージはその検査が読むファイルだけを
  コピーして実行するので、それらが変わらなければ層のキャッシュが当たり、検査は実行されない。層は GitHub Actions の
  キャッシュにジョブごとに残す。
- 必須のチェックは、すべてのジョブの結果をまとめる `check` ジョブ1つ。ジョブを足しても ruleset は変えない。
- キャッシュが当たると新しく公開された脆弱性を拾えないので、週に1回、`check-go` だけキャッシュを使わずに実行する。

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
