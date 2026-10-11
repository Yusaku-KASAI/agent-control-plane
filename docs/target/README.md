# ターゲット

このリポジトリを、エージェントが作業するターゲットとして成り立たせる環境（devcontainer、Claude Code の設定、CI、
作業の進め方）と、ターゲットの契約（[contract.md](contract.md)）。target-template の元になる。

## devcontainer

Anthropic の [Claude Code devcontainer](https://github.com/anthropics/claude-code/tree/main/.devcontainer) を元にした、
人の開発環境とエージェントの実行環境を兼ねるコンテナ（`.devcontainer/`）。

### 使い方

devcontainer のビルド、起動、停止はコマンドで行う。VS Code は起動しているコンテナに入るだけで、ライフサイクルには
関わらない（「Rebuild Container」は使わない）。

```bash
.devcontainer/init-host.sh                                   # ホストの準備（最初に一度。何度実行してもよい）
docker compose -f .devcontainer/compose.yaml up -d --build   # ビルドして起動する
docker compose -f .devcontainer/compose.yaml down            # 止める
```

起動したあと、VS Code の「Reopen in Container」で入る。

`init-host.sh` は次を行い、`.devcontainer/.env` にすでにある値は変えない。

- `.devcontainer/.env`（gitignore の対象。項目は `.devcontainer/.env.example`）に、ホストから決めた値を書く。
  リポジトリ名、UID と GID、タイムゾーン、状態を置くディレクトリ（既定は `~/.local/state/devcontainers/<リポジトリ名>`）。
- 状態を置くディレクトリと、control-plane と共有するネットワーク（`agent-control-plane-net`）を作る。状態の
  ディレクトリがないまま起動すると、Docker が root の持ち物として作り、root でないコンテナから書き込めない。

compose.yaml と devcontainer.json は、名前やパスを `.env` の値とフォルダ名から決めるので、ターゲットが変わっても
書き換えない。

### 方針

- コンテナの振る舞いは Dockerfile、compose.yaml、entrypoint で決める。devcontainer.json は VS Code 側の
  配線だけを持つ（[ADR 0001](adr/0001-devcontainer-independent-of-vscode.md)）。
- `base` ステージには、リポジトリを問わずエージェントが使うもの（Node、Python と uv、シェルツール、Claude Code など）
  だけを入れ、リポジトリやホストに依存する値（リポジトリ名、タイムゾーン、UID）を含めない。層は変わりにくいものから
  順に並べる。リポジトリ固有のツールチェーンとホストに依存する値は後段のステージに置く。
- RUN は `pipefail` 付きの bash で動かし、パイプの途中の失敗（ダウンロードの失敗など）でビルドを止める。
- 外から取得するもの（ベースイメージ、Claude Code、ツール）の版は Dockerfile で固定する。上げるときは PR で変え、
  CI でイメージのビルドと検査を確かめる。Claude Code はネイティブのインストーラーで入れ、自分で更新しないよう
  `DISABLE_UPDATES=1` にする。
- `node` には sudo を与えない。apt のパッケージ一覧はイメージに残し、人が `docker exec -u root` で入って一時的に
  インストールできるようにする（[ADR 0002](adr/0002-no-sudo-for-agent-user.md)）。コンテナの中にファイアウォールは
  置かない（[ADR 0008](adr/0008-no-in-container-firewall.md)）。

### 構成

- devcontainer のイメージは `base` → `go`（このリポジトリのツールチェーンとホストに依存する値）で、
  `<リポジトリ名>-devcontainer` という名前でビルドされる。control-plane はエージェントも同じイメージで動かす
  （`acp.yaml` の `target.image`）。Go のツールは base と切り離した `go-tools` ステージでビルドし、バイナリだけを
  `go` にコピーする。base を変えてもツールを作り直さない。CI 用の `check-*` ステージは「CI」の節にある。
- ビルドの文脈は `.devcontainer/Dockerfile.dockerignore` で決める（ルートの `.dockerignore` は control-plane の
  イメージ用）。
- `..` を `/workspace/<リポジトリ名>` に読み書き可、`../..` を `/workspace` に**読み取り専用**で
  マウントする。隣に置いたリポジトリは参照できるが書き込めない。
- Claude Code の設定（ログイン、履歴、メモリ）、`gh` の設定、シェルの履歴は、ホストの `DEV_STATE_DIR` の下に置いて
  マウントする。Docker の外にあるので、`docker compose down -v` や prune でも消えない
  （[ADR 0010](adr/0010-devcontainer-state-on-the-host.md)）。VS Code のサーバー（拡張機能とチャットの履歴を含む）は、コンテナを作り直しても残るよう名前付きボリュームに置く。
  Go のキャッシュは消えても困らないので、名前付きボリュームに置く。
- コンテナは root を使わず、数値の UID と GID の `node` で動く（[ADR 0009](adr/0009-containers-run-as-non-root.md)）。
  UID と GID は、ビルドのときにリポジトリの所有者の値（`.devcontainer/.env`）に合わせる。control-plane の
  `.env` の `ACP_UID` / `ACP_GID` も同じ値にする。ボリュームのマウント先はイメージの中に `node` の持ち物として
  作ってあり、新しい名前付きボリュームはその所有者を引き継ぐ。root で作業したいときは `docker exec -u root` で入る。

### Claude Code の設定

`.claude/settings.json` は人が対話で使うときの設定。エージェントは `--dangerously-skip-permissions` で動くので、
`permissions` は効かない。コンテナの隔離で守る方針なので、対話でも権限の確認を省いて（bypass）使ってよい。
`permissions.allow` の一覧は、組織の制限などで bypass が使えない環境で確認を減らすためにある。

### ターミナル

- VS Code の既定のターミナルは tmux。VS Code やリモートの接続が切れても、中で動いている Claude Code は止まらない。
- tmux のコピーモードは普段の端末の選択に近づけている。マウスでの選択（ドラッグ、ダブルクリック、トリプルクリック）は
  選択するだけで自動ではコピーせず、`y`、`Enter`、`Ctrl+C` でコピーする。`Esc` かクリックで抜ける。
- Claude Code のフルスクリーン表示では、マウスでの選択を Claude Code が扱い、離したときに自動でコピーする。
  止めるには `/config` の Copy on select をオフにする（そのあとは選択中に `Ctrl+C` か `Ctrl+Shift+C` でコピーする）。
- Claude Code が対話で止まったときは、platform の bell プラグインがベルを鳴らす（[platform](../../platform/docs/README.md#bell)）。

### VS Code の設定

エディタ設定は `devcontainer.json` の `customizations.vscode.settings`（コンテナに attach したウィンドウ）と
`.vscode/settings.json`（attach していないウィンドウ）に同じ内容で置く。ベル設定のように、片方だけだと
効いたり効かなかったりするものがあるため。同じであることは `make check-target` で確かめる。

## ターゲットの契約

ターゲットが用意するもの（イメージの要件、プラグインとロールの宣言、検査、CLAUDE.md の節、ファイルの分類）は
[contract.md](contract.md) にある。

## CI

PR と main への push ごとに、devcontainer のイメージの中で `make check` を実行する
（[ADR 0005](adr/0005-ci-runs-in-devcontainer-image.md)）。あわせて control-plane のイメージがビルドできることを
確かめる（`check-image`）。

- `make check` は関心ごとの検査（`check-go`、`check-platform`、`check-target`、`check-docs`）をまとめたもの
  （[ADR 0011](adr/0011-ci-checks-as-cached-build-stages.md)）。検査が読むファイルを増やしたら、Dockerfile の
  そのステージのコピーにも足す。
- CI は関心ごとのジョブ（`check-go`、`check-platform`、`check-target`、`check-docs`）で、Dockerfile の同じ名前のステージを、
  イメージを書き出さずにビルドする（`check-image` も書き出さない）。各ステージはその検査が読むファイルだけを
  コピーして実行するので、それらが変わらなければ層のキャッシュが当たり、検査は実行されない。層は GitHub Actions の
  キャッシュにジョブごとに残す。
- ジョブを足しても ruleset は変えない（必須のチェックは `check` だけ。[契約](contract.md#検査)）。
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
