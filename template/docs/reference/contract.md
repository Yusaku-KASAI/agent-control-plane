# ターゲットの契約

ターゲットが用意するもの。control-plane と platform はこれを前提にする（[conventions ADR 0002](../../../platform/plugins/conventions/docs/adr/0002-documents-by-distribution-context.md)）。
control-plane がコンテナに渡すものは、逆向きの契約として platform の[ハーネスの契約](../../../platform/docs/reference/harness.md)にある。

<a id="image"></a>

## イメージ

control-plane は、ターゲットのイメージ（ターゲットの devcontainer のイメージ）でエージェントの
コンテナを作る。イメージは次を満たす（[ADR 0009](../adr/0009-containers-run-as-non-root.md)）。

- イメージのユーザーが root でなく、作業ディレクトリ（リポジトリのマウント先）に書き込める。
- `claude`、`git`、`gh` が入っている。
- 引数に渡したコマンド（`claude ...`）をそのまま実行できる。entrypoint があれば、準備を済ませたあと引数のコマンドを
  `exec` する。
- entrypoint は標準入力をそのまま渡し、自身の出力は stderr に出す。stdout はエージェントの回答として扱われる。

<a id="host"></a>

## ホストでの配置

- ターゲットと、ターゲットが宣言するマーケットプレイスは、ホストの同じ親ディレクトリに並べる。その親ディレクトリが
  devcontainer では `/workspace` になる（ターゲットは `/workspace/<ターゲットのディレクトリ名>`）。
- platform のマーケットプレイスは、その親ディレクトリに `agent-platform` という名前で置く。
- 並べるのは相対パスのシンボリックリンクでもよい。

<a id="plugins"></a>

## プラグインとロール

- 使うプラグインは `.claude/settings.json` の `extraKnownMarketplaces` と `enabledPlugins` で宣言する
  （[ADR 0004](../adr/0004-plugins-declared-in-settings.md)）。
  - `directory` のマーケットプレイスの場所は、devcontainer でのパス `/workspace/<リポジトリ>/...` で書く。platform は
    `/workspace/agent-platform` で宣言する（[ADR 0007](../adr/0007-marketplaces-beside-the-target.md)）。
  - 宣言したマーケットプレイスがエージェントにどう見えるかは、platform の[ハーネスの契約](../../../platform/docs/reference/harness.md)にある。
- 有効にするプラグインが前提にするプラグインも有効にする（何を前提にするかは、各プラグインの文書にある）。
- ロールは Claude Code のカスタムエージェントとして定義する（[ADR 0003](../adr/0003-roles-as-claude-code-agents.md)）。
  ターゲットの `.claude/agents/<ロール名>.md` に platform のロールと同名の定義を置くと、そちらが優先される。
  定義が見つからないロールで起動すると、エージェントはエラーで終了する。
- ロールの定義には振る舞いだけを書き、ターゲット固有の事実や規約は CLAUDE.md に書く。

<a id="checks"></a>

## 検査

- `make check` が、そのターゲットの検査をすべて行う。関心ごとに `check-<関心>` に分け、共通の `check-target`
  （devcontainer のスクリプトと設定）と `check-docs`（確かめるものは conventions の[文書の形式](../../../platform/plugins/conventions/docs/reference/formats.md)の各節）を含める。
- CI は `check-<関心>` ごとのジョブで、devcontainer の Dockerfile の同じ名前のステージをビルドし、結果を
  `check` ジョブにまとめる（[ADR 0005](../adr/0005-ci-runs-in-devcontainer-image.md)、
  [ADR 0011](../adr/0011-ci-checks-as-cached-build-stages.md)）。必須のチェックは `check` だけにする。

## CLAUDE.md

エージェントに常に読み込まれる入口（[ADR 0012](../adr/0012-claude-md-sections.md)）。次の節をこの順に持ち、どの節も
短くして、詳しいことは文書へのリンクにする。当てはまらない節は「なし」と書いて残す。

| 節 | 書くこと |
| --- | --- |
| 冒頭（見出しの下） | 何のためのリポジトリか（1〜2 文） |
| 使われ方 | 誰が、どこで、どう使うか |
| 公開と運用 | どこに、どう公開・配布しているか（本番の環境、デプロイの仕方）。手元の動かし方は文書へのリンク |
| 守る互換 | 壊してはいけないもの（公開した URL やデータの形式、API など）と、破壊的に変えてよい範囲 |
| 参照先 | 文書の入口へのリンクと、それぞれ何があるか |
| 作業の完了条件 | `make check` など、変更後に通すもの |
| 言語 | ドキュメント、コミットメッセージ、コード中のコメントの言語 |
| 共通のルール | platform の conventions プラグインが読み込ませること（文面は下のとおり） |

共通のルールの節の文面：

```markdown
## 共通のルール

コメントとドキュメントの書き方など、どのターゲットでも同じルールは、`.claude/settings.json` で有効にしている
platform の conventions プラグインが、セッションとサブエージェントの始めに読み込ませる。
```

ほかに、そのターゲットだけの決まり（ほかのリポジトリの扱いなど）は、この後に節を足す。

<a id="files"></a>

## ファイルの分類

ターゲットの環境を作るファイルは、target-template と比べて次の 3 つに分かれる。

| 分類 | 意味 |
| --- | --- |
| 共通 | どのターゲットでも同じ。ターゲットで変えない。変えるときは target-template とすべてのターゲットで変える |
| 埋める | 形は共通で、ターゲットごとの部分を持つ。共通の部分は変えず、決まった場所だけを埋める |
| 固有 | ターゲットだけのもの |

| ファイル | 分類 | ターゲットごとの部分 |
| --- | --- | --- |
| `.devcontainer/docker-entrypoint.sh`、`init-host.sh`、`tmux.conf`、`check-vscode-settings.py` | 共通 | — |
| `tools/check-docs.py` | 共通 | — |
| `.devcontainer/Dockerfile` | 埋める | ツールチェーンのステージ（`base` の次）と `check-<関心>` のステージ。`base` と `check-base`、`check-target`、`check-docs` は共通 |
| `.claude/settings.json` | 埋める | 使うプラグインの宣言（`extraKnownMarketplaces`、`enabledPlugins`）と、対話のときの権限（`permissions`） |
| `.devcontainer/compose.yaml` | 埋める | ビルドするステージの名前と、ツールチェーンのキャッシュのボリューム |
| `.devcontainer/devcontainer.json`、`.vscode/settings.json` | 埋める | ツールチェーンの拡張機能とその設定 |
| `.devcontainer/Dockerfile.dockerignore` | 埋める | ツールチェーンが作るものの除外 |
| `.devcontainer/.env.example` | 埋める | リポジトリ名を含む値 |
| `.github/workflows/ci.yml` | 埋める | 行列のジョブ（`check-<関心>` と、ターゲットのほかのイメージのビルド）と、定期実行でキャッシュを使わない検査 |
| `Makefile` | 埋める | `check` が束ねる `check-<関心>` とその中身。`check-target` と `check-docs` は共通 |
| `.gitignore` | 埋める | `.devcontainer/.env` の行は共通、ほかはターゲットごと |
| `CLAUDE.md` | 埋める | 上の節の中身 |
| それ以外（アプリのコード、文書、ADR） | 固有 | — |

共通と埋めるファイルの共通の部分が target-template と食い違っていないことは、検査では確かめない。
