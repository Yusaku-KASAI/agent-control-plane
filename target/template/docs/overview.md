# target-template の全体像

[ターゲット](../../../platform/docs/reference/glossary.md)に共通の環境のファイルと、その構成。ターゲットが用意する
ものは[ターゲットの契約](../../../platform/docs/reference/target.md)が定め、ここにあるファイルはそれを満たす。ひな形のファイルは
写す元の `target/`（この文書の根の二つ上）にあり、新しいターゲットは[新しいターゲットを作る](guides/new-target.md "在りか")の
とおりに作り、[ひな形の新しい版に合わせる](guides/update-target.md "在りか")。

<a id="parts"></a>

## 構成

target-template は、ターゲットの環境の部分（表の上の 4 行）と、それをターゲットに配る仕組み（表の最後の行）からなる。

| 部分 | 中身 | 文書 |
| --- | --- | --- |
| devcontainer | `.devcontainer/`（イメージ、compose、VS Code の配線、ホストの準備）、`.vscode/settings.json`、`.gitignore` の `.devcontainer/.env` の行 | [architecture/devcontainer.md](architecture/devcontainer.md "在りか")、[guides/devcontainer.md](guides/devcontainer.md "在りか") |
| Claude Code の設定 | `.claude/settings.json`（プラグインの宣言、回答の言語、worktree の作り方、対話のときの権限）、`.gitignore` の Claude Code の行 | [architecture/claude-code.md](architecture/claude-code.md "在りか")、[guides/claude-code.md](guides/claude-code.md "在りか") |
| CI | `Makefile` の `check-<関心>`、`.github/workflows/ci.yml` | [architecture/ci.md](architecture/ci.md "在りか") |
| 既定のブランチの守り | `.github/CODEOWNERS`（承認する人）と、リポジトリの設定のルールセット | [guides/protection.md](guides/protection.md "在りか") |
| ひな形の配り方 | 写す元の `target/` から写し、新しい版に合わせる道具（`template/bin/`）、入口と文書の根のひな形（`CLAUDE.md`、`README.md`、`docs/` の `.tmpl`）、写したターゲットが持つ `template/docs/` と `template/base` | [guides/new-target.md](guides/new-target.md "在りか")、[guides/update-target.md](guides/update-target.md "在りか") |

<a id="files"></a>

## ファイルの分類

ターゲットのファイルは、target-template と比べて次の 3 つに分かれる。

| 分類 | 意味 |
| --- | --- |
| 共通 | どのターゲットでも同じ。ターゲットで変えない。変えるときは target-template とすべてのターゲットで変える |
| 埋める | 形は共通で、ターゲットごとの部分を持つ。共通の部分は変えず、決まった場所だけを埋める。下の表のターゲットごとの部分が消すことを挙げていれば、ファイルごと消すことも埋め方に含む |
| 固有 | ターゲットだけのもの |

| ファイル | 分類 | ターゲットごとの部分 |
| --- | --- | --- |
| `.devcontainer/docker-entrypoint.sh`、`init-host.sh`、`tmux.conf`、`check-vscode-settings.py` | 共通 | — |
| `.devcontainer/Dockerfile` | 埋める | ツールチェーンのステージ（`base` を元にするもの）の中身と、それが使うステージ（外のイメージに名前を付けるもの、ツールを切り離してビルドするもの）、`check-base` が元にするステージの名前、`check-<関心>` のステージ。`uv-source`、`base`、ツールチェーンのステージのうちホストに依存する値を足す部分、`check-base`、`check-target` は共通 |
| `.claude/settings.json` | 埋める | 使うプラグインの宣言（`extraKnownMarketplaces`、`enabledPlugins`）、回答の言語（`language`）、対話のときの権限（`permissions`）。`worktree` は共通 |
| `.devcontainer/compose.yaml` | 埋める | ビルドするステージの名前と、ツールチェーンのキャッシュのボリューム |
| `.devcontainer/devcontainer.json`、`.vscode/settings.json` | 埋める | ツールチェーンの拡張機能とその設定 |
| `.devcontainer/Dockerfile.dockerignore` | 埋める | ツールチェーンが作るものの除外 |
| `.devcontainer/.env.example` | 埋める | リポジトリ名を含む値 |
| `.github/workflows/ci.yml` | 埋める | 行列のジョブ（`check-<関心>` と、ターゲットのほかのイメージのビルド）と、定期実行でキャッシュを使わない検査。ワークフローの権限（`permissions: contents: read`）、`check-docs` と `check-protection` のジョブ（その権限を含む）と、`check` ジョブがそれらを待つことは共通 |
| `Makefile` | 埋める | `check` が束ねる `check-<関心>` とその中身。`check-target` は共通 |
| `.gitignore` | 埋める | `.devcontainer/.env`、`/.claude/worktrees/`、`.claude/settings.local.json`、`CLAUDE.local.md` の行は共通、ほかはターゲットごと |
| `.github/CODEOWNERS` | 埋める | 持ち主のユーザー名。organization のリポジトリでは消す（team で守る。[ADR 0015](adr/0015-codeowners-by-default.md)） |
| `CLAUDE.md` | 埋める | 節の中身（節は[ターゲットの契約](../../../platform/docs/reference/target.md#claudemd)にある） |
| それ以外（アプリのコード、文書、ADR） | 固有 | — |

写したターゲットは、環境のファイルのほかに、target-template の文書を `template/docs/` に、写したか合わせた版の
agent-control-plane のコミットを `template/base` に持つ。写すときに、platform と
control-plane へのリンクを GitHub へのリンクに書き換える（[ADR 0020](adr/0020-template-documents-linked-to-github-in-copies.md)）。

ひな形のファイルでは、埋める場所に `Fill in:` で始まるコメントを置く（コメントを書けない `.claude/settings.json` を
除く）。値だけを埋める所（`.devcontainer/.env.example` の `<repository>`、`.github/CODEOWNERS` の `<owner>`）は、`<...>` で示す。Markdown では、
HTML のコメント（`<!-- Fill in: ... -->`）を印にする。

`CLAUDE.md`、`README.md`、`docs/README.md`、`docs/overview.md` のひな形は、名前の終わりに `.tmpl` を足して置き、写したら
外す（[ADR 0019](adr/0019-renamed-templates-for-entry-documents.md)）。`target/` の下の `CLAUDE.md` は agent-control-plane で
その下のファイルを読むエージェントに読み込まれ、`.md` で終わる文書は agent-control-plane の文書として検査されるため。
`README.md` と `docs/` は、ファイルの分類では固有で、ひな形は書き始めの器だけを持つ。

共通と埋めるファイルの共通の部分が target-template と食い違っていないことは、ターゲットの検査では確かめない。

<a id="map-target-template"></a>

## 地図

target-template の中の項目と前提の向き（[形式](../../../platform/plugins/conventions/docs/reference/formats.md#map "在りか")）。
ターゲットの環境は、[構成](#parts)の環境の部分の項目をまとめる、文書を持たない集まりで、その部分の項目もこの地図に書く。
ひな形の配り方は、構成の最後の行の部分。

| 項目 | 親 | 置き場所 | 前提にする |
| --- | --- | --- | --- |
| ターゲットの環境 | target-template | | |
| devcontainer | ターゲットの環境 | `architecture/devcontainer.md`, `guides/devcontainer.md` | |
| Claude Code の設定 | ターゲットの環境 | `architecture/claude-code.md`, `guides/claude-code.md` | devcontainer |
| CI | ターゲットの環境 | `architecture/ci.md` | devcontainer |
| 既定のブランチの守り | ターゲットの環境 | `guides/protection.md` | CI |
| ひな形の配り方 | target-template | `guides/new-target.md`, `guides/update-target.md` | ターゲットの環境 |
