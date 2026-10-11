# devcontainer

人の開発環境とエージェントの実行環境を兼ねるコンテナ（`.devcontainer/`）。Anthropic の
[Claude Code devcontainer](https://github.com/anthropics/claude-code/tree/main/.devcontainer) を元にしている。
使い方は[手順](../guides/devcontainer.md)に、ファイルごとの共通・埋める・固有の分類は[契約](../reference/contract.md)にある。

## 構成

```
init-host.sh ─書く─▶ .env ─読む─▶ compose.yaml ─ビルド・起動─▶ コンテナ ◀─attach─ devcontainer.json
（ホストの準備）    （ホストの値）  （コンテナの振る舞い）                       （VS Code の配線）

Dockerfile のステージ（矢印は元にするか、コピーする向き）
  uv-source ─▶ base ─▶ go ─▶ check-base ─▶ check-<関心>
  go-source ─▶ go-tools ─▶ go
  go-source ─▶ go
```

- コンテナの振る舞いは Dockerfile、compose.yaml、entrypoint で決める。devcontainer.json は VS Code 側の配線だけを
  持ち、ビルド、起動、停止はコマンドで行う（[ADR 0001](../adr/0001-devcontainer-independent-of-vscode.md)）。
- Dockerfile のステージは、変わりにくく共有できるものから順に重ねる。`base` はどのターゲットのエージェントも
  使うもの、その次のツールチェーンのステージ（このリポジトリでは `go`）はリポジトリ固有のツールチェーンとホストに
  依存する値を持つ。devcontainer のイメージはツールチェーンのステージまでで、`<リポジトリ名>-devcontainer` という
  名前でビルドされる。control-plane はエージェントも同じイメージで動かす（`acp.yaml` の `agents[].target.image`）。
  `check-*` のステージは CI が使う（[ci.md](ci.md)）。
- ホストに依存する値（リポジトリ名、UID と GID、タイムゾーン、状態を置くディレクトリ）は、`init-host.sh` が
  `.devcontainer/.env` に書き、compose.yaml がそこから読む。compose.yaml と devcontainer.json はターゲットが
  変わっても書き換えない。
- コンテナは root を使わず、数値の UID と GID の `node` で動く（[ADR 0009](../adr/0009-containers-run-as-non-root.md)）。
  UID と GID は、ビルドのときにリポジトリの所有者の値（`.devcontainer/.env`）に合わせる。root での作業の仕方は[手順](../guides/devcontainer.md#root)にある。
- マウントするもの：

  | もの | 置き場所 | 理由 |
  | --- | --- | --- |
  | リポジトリ（`..`） | `/workspace/<リポジトリ名>` に読み書き可 | |
  | 隣のリポジトリ（`../..`） | `/workspace` に**読み取り専用** | 参照できるが書き込めない |
  | Claude Code の設定（ログイン、履歴、メモリ）、`gh` の設定、シェルの履歴 | ホストの `DEV_STATE_DIR` の下 | Docker の外にあるので、`docker compose down -v` や prune でも消えない（[ADR 0010](../adr/0010-devcontainer-state-on-the-host.md)） |
  | VS Code のサーバー（拡張機能とチャットの履歴を含む） | 名前付きボリューム | コンテナを作り直しても残る |
  | ツールチェーンのキャッシュ（Go のモジュールとビルド） | 名前付きボリューム | 消えても困らない |

  マウント先とその親はイメージの中に `node` の持ち物として作ってあり、新しい名前付きボリュームはその所有者を
  引き継ぐ。

## 構成要素

### `.devcontainer/Dockerfile`

- **責務**：devcontainer とエージェントのイメージ、CI の検査のステージを定義する。
- **置くもの / 置かないもの**：外から取得するもの（ベースイメージ、Claude Code、ツール）とその版。版は ARG で
  固定するので、版を上げる変更は PR になり、CI がイメージのビルドと検査を通す。RUN は `pipefail` 付きの bash で動かし、
  パイプの途中の失敗（ダウンロードの失敗など）でビルドを止める。
- **関係**：compose.yaml と CI がビルドする。ビルドの文脈は `Dockerfile.dockerignore` で決まる。
- **当てはめた原則**：[仕組みで守る](../../../platform/docs/reference/principles.md#mechanisms)：版の固定と `pipefail`、
  入れた Claude Code の版の確認で、意図しない版や途中の失敗をビルドで止める。

### Dockerfile の `uv-source`、`go-source` ステージ

- **責務**：版を固定した外のイメージに名前を付け、`COPY --from` で使えるようにする（`COPY --from` はビルド引数を
  展開できない）。

### Dockerfile の `base` ステージ

- **責務**：リポジトリを問わずエージェントが使うもの（Node、Python と uv、シェルツール、git-delta、zsh、Claude Code
  など）を入れる。
- **置くもの / 置かないもの**：リポジトリやホストに依存する値（リポジトリ名、タイムゾーン、UID）は置かない。層は
  変わりにくいものから順に並べ、tmux.conf と entrypoint のような変わりやすい小さなファイルは最後にコピーする。
  - `node` には sudo を与えない。apt のパッケージ一覧はイメージに残し、人が `docker exec -u root` で入って一時的に
    インストールできるようにする（[ADR 0002](../adr/0002-no-sudo-for-agent-user.md)）。
  - コンテナの中にファイアウォールは置かない（[ADR 0008](../adr/0008-no-in-container-firewall.md)）。
  - Claude Code はネイティブのインストーラーで入れ、自分で更新しないよう `DISABLE_UPDATES=1` にする。設定が
    マウントしたディレクトリの外（`~/.claude.json`）に残らないよう、`CLAUDE_CONFIG_DIR` を `~/.claude` にする。
  - git-delta は system の gitconfig で設定する。VS Code がホストから持ち込む `~/.gitconfig` に上書きされない。
- **関係**：ツールチェーンのステージがこれを元にする。どのターゲットでも同じ内容（[契約](../reference/contract.md)）。
- **当てはめた原則**：[依存の向き](../../../platform/docs/reference/principles.md#dependency-rule)：共有される中心なので、
  リポジトリやホストを知らない。後段がこれに足す。
  [高凝集・疎結合](../../../platform/docs/reference/principles.md#cohesion)：変わりにくいものから層を重ね、変わりやすい
  ものを変えても前の層のキャッシュが当たる。

### Dockerfile の `go-tools` ステージ

- **責務**：Go のツール（gopls、delve、govulncheck、golangci-lint）を、base と切り離してビルドする。
- **関係**：バイナリだけを `go` の `/usr/local/bin` にコピーする。GOPATH の下は一部がボリュームなので置かない
  （版がここで固定したものからずれる）。
- **当てはめた原則**：[高凝集・疎結合](../../../platform/docs/reference/principles.md#cohesion)：base を変えても
  （Claude Code を上げても）ツールを作り直さない。

### Dockerfile の `go` ステージ（ツールチェーンのステージ）

- **責務**：このリポジトリのツールチェーン（Go とそのツール）と、ホストに依存する値（タイムゾーン、UID と GID）を
  base に足す。devcontainer のイメージになる。
- **置くもの / 置かないもの**：UID と GID が `node` と違えば `node` を作り直して持ち物を付け替え、最後に数値の
  `USER` にする（数値なので、プラットフォームが root でないことを確かめられる）。
- **関係**：compose.yaml がこのステージをビルドする。`check-base` がこれを元にする。ターゲットごとに埋める部分
  （[契約](../reference/contract.md)）。
- **当てはめた原則**：[関心の分離](../../../platform/docs/reference/principles.md#soc)：リポジトリとホストに固有のものを、
  共有する base から分ける。

### `.devcontainer/compose.yaml`

- **責務**：devcontainer のコンテナの振る舞い（ビルドするステージ、イメージの名前、マウント、環境変数、
  ネットワーク）を決める。
- **置くもの / 置かないもの**：名前とパスは `.env` の値から決め、値そのものは置かない。値がなければ `init-host.sh`
  を実行するよう言って止まる。
  - control-plane と共有するネットワーク `agent-control-plane-net`（外部のネットワーク）につなぎ、`ACP_SERVER` で
    control-plane を指す。API のトークンは渡さない。渡すと、このコンテナから GitHub の認証情報を持つエージェントを
    起動できてしまう。
- **関係**：`.env` を読み、Dockerfile のツールチェーンのステージをビルドする。devcontainer.json がこれを参照する。
- **当てはめた原則**：[単一の情報源](../../../platform/docs/reference/principles.md#ssot)：ホストの値は `.env` にだけ置く。
  [関心の分離](../../../platform/docs/reference/principles.md#soc)：コンテナの振る舞いを VS Code の配線から分ける。

### `.devcontainer/devcontainer.json`

- **責務**：起動しているコンテナに VS Code が入るための配線と、拡張機能とエディタ設定を持つ。
- **置くもの / 置かないもの**：ライフサイクルに関わらない設定（`overrideCommand: false`、`shutdownAction: none`、
  `runServices: []`）。VS Code が UID を変えないよう `updateRemoteUserUID: false` にする。コンテナの振る舞いは置かない。
- **関係**：compose.yaml のサービスに attach する。エディタ設定は `.vscode/settings.json` と同じ内容にする。
- **当てはめた原則**：[関心の分離](../../../platform/docs/reference/principles.md#soc)：VS Code がなくてもコンテナは同じに
  動く（[ADR 0001](../adr/0001-devcontainer-independent-of-vscode.md)）。

### `.devcontainer/init-host.sh`

- **責務**：ホストを devcontainer のために準備する（`.env`、状態を置くディレクトリ、共有のネットワーク）。
- **置くもの / 置かないもの**：何度実行しても同じになる処理。`.env` にすでにある値は変えない。何をするかは
  [手順](../guides/devcontainer.md)にある。
- **関係**：`.env.example` の項目を `.env` に書く。
- **当てはめた原則**：[仕組みで守る](../../../platform/docs/reference/principles.md#mechanisms)：状態のディレクトリを先に
  作る。ないまま起動すると Docker が root の持ち物として作り、root でないコンテナから書き込めない。

### `.devcontainer/.env.example`

- **責務**：compose.yaml が読む `.env` の項目とその意味を示す。
- **置くもの / 置かないもの**：項目と例の値。実際の値は gitignore の対象の `.devcontainer/.env` に置く。
- **関係**：`init-host.sh` が同じ項目を書き、compose.yaml が読む。
- **当てはめた原則**：[スキーマを先に](../../../platform/docs/reference/principles.md#schema-first)：`.env` の形を例として
  先に置く。

### `.devcontainer/docker-entrypoint.sh`

- **責務**：イメージのユーザーが作業ディレクトリに書き込めないときにそう知らせ、引数のコマンドを `exec` する。
- **置くもの / 置かないもの**：知らせは stderr に出す。標準入力と stdout には触らない。
- **関係**：[契約](../reference/contract.md)のイメージの entrypoint の決まりに沿う。
- **当てはめた原則**：[仕組みで守る](../../../platform/docs/reference/principles.md#mechanisms)：UID の食い違いを、ツールが
  後で権限で失敗する前に知らせる。

### `.devcontainer/tmux.conf`

- **責務**：ターミナルの tmux の振る舞いを決める。
- **置くもの / 置かないもの**：
  - VS Code の既定のターミナルは tmux。VS Code やリモートの接続が切れても、中で動いている Claude Code は止まらない。
  - コピーモードは普段の端末の選択に近づけている。マウスでの選択（ドラッグ、ダブルクリック、トリプルクリック）は
    選択するだけで自動ではコピーせず、`y`、`Enter`、`Ctrl+C` でコピーする。`Esc` かクリックで抜ける。
  - Claude Code のフルスクリーン表示では、マウスでの選択を Claude Code が扱い、離したときに自動でコピーする。
    止め方は[手順](../guides/devcontainer.md#copy)にある。
  - ペインのベルは外の端末に届け、tmux にも表示する。Claude Code が対話で止まったときは、platform の bell プラグインが
    ベルを鳴らす（[bell](../../../platform/docs/architecture/plugins.md#bell)）。
- **関係**：base が `~/.tmux.conf` にコピーする。VS Code のターミナルのプロファイルが tmux を起動する。

### `.devcontainer/check-vscode-settings.py`

- **責務**：devcontainer.json の `customizations.vscode.settings` と `.vscode/settings.json` が同じであることを確かめる。
- **置くもの / 置かないもの**：JSONC なので、インデントと行全体のコメントを除いて行ごとに比べる。
- **関係**：`make check-target` が実行する。
- **当てはめた原則**：[仕組みで守る](../../../platform/docs/reference/principles.md#mechanisms)：同じ設定を二か所に置く
  しかないので、食い違いを検査で見つける。

### `.devcontainer/Dockerfile.dockerignore`

- **責務**：この Dockerfile のビルドの文脈から、手元の秘密（`.env` など）とビルドの出力を除く。
- **関係**：ルートの `.dockerignore` は control-plane のイメージ用で、この Dockerfile には使われない。`check-*` の
  ステージはこの文脈からコピーする。
- **当てはめた原則**：[関心の分離](../../../platform/docs/reference/principles.md#soc)：イメージごとに文脈を分ける。

### `.claude/settings.json`

- **責務**：使うプラグインの宣言と、人が対話で使うときの Claude Code の設定を持つ。
- **置くもの / 置かないもの**：
  - プラグインは `extraKnownMarketplaces` と `enabledPlugins` で宣言する（[契約](../reference/contract.md)）。
  - `permissions` は人が対話で使うときだけ効く。エージェントは `--dangerously-skip-permissions` で動くので効かない。
    コンテナの隔離で守る方針なので、対話でも権限の確認を省いて（bypass）使ってよい。`permissions.allow` の一覧は、
    組織の制限などで bypass が使えない環境で確認を減らすためにある。
- **関係**：control-plane がプラグインの宣言を読む（[ADR 0004](../adr/0004-plugins-declared-in-settings.md)）。
- **当てはめた原則**：[境界づけられた文脈](../../../platform/docs/reference/principles.md#bounded-context)：プラグインの
  宣言は control-plane との契約（Published Language）で、使う側も読める決まった場所に置く。

### `.vscode/settings.json`

- **責務**：コンテナに attach していない VS Code のウィンドウのエディタ設定を持つ。
- **置くもの / 置かないもの**：devcontainer.json の `customizations.vscode.settings`（attach したウィンドウ）と同じ内容。
  ベル設定のように、片方だけだと効いたり効かなかったりするものがあるため。ツールはイメージで固定するので、拡張機能が
  自分でツールを入れないようにする。
- **関係**：同じであることは `make check-target`（`check-vscode-settings.py`）で確かめる。
- **当てはめた原則**：[仕組みで守る](../../../platform/docs/reference/principles.md#mechanisms)：二か所に置く同じ設定を
  検査でそろえる。
