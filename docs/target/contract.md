# ターゲットの契約

ターゲットが用意するもの。control-plane と platform はこれを前提にする（[platform ADR 0005](../../platform/docs/adr/0005-documents-by-distribution-context.md)）。
control-plane がコンテナに渡すものは、逆向きの契約として platform の[ハーネスの契約](../../platform/docs/harness.md)にある。

## イメージ

control-plane はセッションごとにターゲットのイメージからコンテナを作り、`claude -p` を `stream-json` の
入出力で動かし続けて、依頼を標準入力に送る。そのためにイメージは次を満たす。

- イメージのユーザーが root でなく、作業ディレクトリ（リポジトリのマウント先）に書き込める。control-plane は
  `claude ...` をそのままコマンドとして渡す（Claude Code は root では権限確認を省略できない）。
- `claude`、`git`、`gh` が入っている。
- entrypoint があれば、準備を済ませたあと引数のコマンドを `exec` する。標準入力はそのまま渡し、自身の出力は
  stderr に出す。stdout はエージェントの回答として扱われる。

## プラグインとロール

- 使うプラグインは `.claude/settings.json` の `extraKnownMarketplaces` と `enabledPlugins` で宣言する
  （[ADR 0004](adr/0004-plugins-declared-in-settings.md)）。control-plane は、`directory` のマーケットプレイスから
  有効なプラグインを `--plugin-dir` で読み込ませる。それ以外のマーケットプレイスのプラグインは Claude Code に任せる。
  - マーケットプレイスの場所は、devcontainer でのパス `/workspace/<リポジトリ>/...` で書く。platform は
    `/workspace/agent-platform` で宣言する。
  - ターゲット自身の中を指すパスは、エージェントのコンテナではセッションの作業領域（作業中のブランチ）を指す。
    隣のリポジトリを指すパスは、control-plane がホストでターゲットの隣にあるそのディレクトリだけを、同じパスに
    読み取り専用でマウントする。ただし、ホストでシンボリックリンクをたどった先がターゲット自身の中なら、作業領域の
    中として扱う（[ADR 0007](adr/0007-marketplaces-beside-the-target.md)）。
- platform の roles を使うなら、それが前提にする harness と review も有効にする。
- ロールは Claude Code のカスタムエージェントとして定義し、control-plane は `claude -p --agent <ロール名>` で
  起動する（[ADR 0003](adr/0003-roles-as-claude-code-agents.md)）。ターゲットの `.claude/agents/<ロール名>.md` に
  platform のロールと同名の定義を置くと、そちらが優先される。定義が見つからないロールで起動すると、エージェントは
  エラーで終了する。
- ロールの定義には振る舞いだけを書き、ターゲット固有の事実や規約は CLAUDE.md に書く。

## 検査

- `make check` が、そのターゲットの検査をすべて行う。関心ごとに `check-<関心>` に分け、`check-target`
  （devcontainer のスクリプトと設定）を含める。
- CI は `check-<関心>` ごとのジョブで、devcontainer の Dockerfile の同じ名前のステージをビルドし、結果を
  `check` ジョブにまとめる（[ADR 0005](adr/0005-ci-runs-in-devcontainer-image.md)、
  [ADR 0011](adr/0011-ci-checks-as-cached-build-stages.md)）。必須のチェックは `check` だけにする。

## CLAUDE.md

エージェントに常に読み込まれる入口（[ADR 0012](adr/0012-claude-md-sections.md)）。次の節をこの順に持ち、どの節も
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
| 共通のルール | platform の rules プラグインが読み込ませること（文面は下のとおり） |

共通のルールの節の文面：

```markdown
## 共通のルール

コメントとドキュメントの書き方など、どのターゲットでも同じルールは、`.claude/settings.json` で有効にしている
platform の rules プラグインが、セッションとサブエージェントの始めに読み込ませる。
```

ほかに、そのターゲットだけの決まり（このリポジトリの「他のリポジトリ」など）は、この後に節を足す。

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
| `.claude/settings.json` | 共通 | — |
| `.devcontainer/Dockerfile` | 埋める | ツールチェーンのステージ（`base` の次）と `check-<関心>` のステージ。`base` と `check-base`、`check-target` は共通 |
| `.devcontainer/compose.yaml` | 埋める | ビルドするステージの名前と、ツールチェーンのキャッシュのボリューム |
| `.devcontainer/devcontainer.json`、`.vscode/settings.json` | 埋める | ツールチェーンの拡張機能とその設定 |
| `.devcontainer/Dockerfile.dockerignore` | 埋める | ツールチェーンが作るものの除外 |
| `.devcontainer/.env.example` | 埋める | リポジトリ名を含む値 |
| `.github/workflows/ci.yml` | 埋める | 行列の `check-<関心>` と、アプリのイメージの検査 |
| `Makefile` | 埋める | `check` が束ねる `check-<関心>` とその中身。`check-target` は共通 |
| `.gitignore` | 埋める | `.devcontainer/.env` の行は共通、ほかはターゲットごと |
| `CLAUDE.md` | 埋める | 上の節の中身 |
| それ以外（アプリのコード、文書、ADR） | 固有 | — |

共通と埋めるファイルの共通の部分が target-template と食い違っていないことは、まだ検査していない。
