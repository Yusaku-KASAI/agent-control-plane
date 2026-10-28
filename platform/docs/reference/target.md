# ターゲットの契約

ターゲットが用意するもの。control-plane と platform はこれを前提にする（[ADR 0031](../adr/0031-contracts-where-every-agent-reads-them.md)）。
control-plane がコンテナに渡すものは、逆向きの契約として[ハーネスの契約](harness.md "在りか")にある。

<a id="image"></a>

## イメージ

control-plane は、ターゲットのイメージ（ターゲットの devcontainer のイメージ）でエージェントの
コンテナを作る。イメージは次を満たす（[target-template の ADR 0009](../../../template/docs/adr/0009-containers-run-as-non-root.md)）。

- イメージのユーザーは root でない `node` で、ホームは `/home/node`。作業ディレクトリ（リポジトリのマウント先）に書き込める。
  管理者の権限（sudo）を持たない。
- 次の道具が `PATH` にある。`claude`、`git`、`gh`（エージェントが使う）と、`bash`、`jq`、`python3`、`curl`、`make`、
  coreutils、`grep`、`sed`、`awk`、findutils、procps の `ps`、util-linux の `flock`（platform のプラグインのコマンドと hook、検査が使う）。
- `claude` は、platform の hook が拠る入力を渡す版にする：サブエージェントを呼ぶツールの名前が `Agent` で、サブエージェント
  自身の呼び出しに `agent_id` が付く（[ADR 0032](../adr/0032-hooks-match-the-agent-tool-name-only.md)。2.1.292 で確かめた）。
  版を上げたら、サブエージェントの呼び出しに hook が働くことを確かめる。
- 引数に渡したコマンド（`claude ...`）をそのまま実行できる。entrypoint があれば、準備を済ませたあと引数のコマンドを
  `exec` する。
- entrypoint は標準入力をそのまま渡し、自身の出力は stderr に出す。stdout はエージェントの回答として扱われる。
- control-plane は、セッションの片付けで作業領域を読むときにも、同じイメージでコンテナを起こす。そのコンテナでは
  entrypoint を control-plane のバイナリに上書きし、読み取り専用のルート、ネットワークなし、イメージのユーザーで
  `git` を動かす。その形でも `git` が動く。

<a id="host"></a>

## ホストでの配置

- ターゲットと、ターゲットが宣言するマーケットプレイスは、ホストの同じ親ディレクトリに並べる。その親ディレクトリが
  devcontainer では `/workspace` になる（ターゲットは `/workspace/<ターゲットのディレクトリ名>`）。
- platform のマーケットプレイスは、その親ディレクトリに `agent-platform` という名前で置く。
- 並べるのは相対パスのシンボリックリンクでもよい。

<a id="plugins"></a>

## プラグイン、ロール、Claude Code の設定

- 使うプラグインは `.claude/settings.json` の `extraKnownMarketplaces` と `enabledPlugins` で宣言する
  （[target-template の ADR 0004](../../../template/docs/adr/0004-plugins-declared-in-settings.md)）。
  - `directory` のマーケットプレイスの場所は、devcontainer でのパス `/workspace/<リポジトリ>/...` で書く。platform は
    `/workspace/agent-platform` で宣言する（[target-template の ADR 0007](../../../template/docs/adr/0007-marketplaces-beside-the-target.md)）。
  - 宣言したマーケットプレイスがエージェントにどう見え、どの宣言が効くかは[ハーネスの契約](harness.md#work-area "在りか")にある。
  - `.claude/settings.json` とマーケットプレイスの `.claude-plugin/marketplace.json` は、通常のファイルで、1 MiB 以内に
    する。シンボリックリンクは、相対パスで、リポジトリ（隣のマーケットプレイスならその中）の中に留まるものだけにする。control-plane
    は、作業領域をエージェントが書けるので、外れたものを読まずに依頼を失敗させる
    （[control-plane の ADR 0044](../../../docs/adr/app/0044-read-workspace-files-confined.md "在りか")）。
- 有効にするプラグインが前提にするプラグインも有効にする（何を前提にするかは、各プラグインの文書にある）。
- `.claude/settings.json` の `language` に、CLAUDE.md の言語の節で決めた回答の言語を書く。
- ロールは Claude Code のカスタムエージェントとして定義する（[target-template の ADR 0003](../../../template/docs/adr/0003-roles-as-claude-code-agents.md)）。
  ターゲットの `.claude/agents/<ロール名>.md` に platform のロールと同名の定義を置くと、そちらが優先される。
  定義が見つからないロールで起動すると、エージェントはエラーで終了する。
- ロールの定義には振る舞いだけを書き、ターゲット固有の事実や規約は CLAUDE.md に書く。定義の形は conventions の
  [形式：役割ごとの指示](../../plugins/conventions/docs/reference/formats.md#role "在りか")に沿う。
- `.claude/settings.json` の `worktree.baseRef` を `head` にし、`.gitignore` で `/.claude/worktrees/` を ignore する
  （[ADR 0016](../adr/0016-subagents-leave-the-work-area.md)）。

<a id="checks"></a>

## 検査

- `make check` が、そのターゲットの検査をすべて行う。エージェントは変更のあとにこれを通す（CLAUDE.md の作業の完了条件）。

<a id="control-plane"></a>

## devcontainer と control-plane

ターゲットの devcontainer は、エージェントのコンテナと同じく、control-plane の Docker ネットワークに入らず、
control-plane の API を指さない（[target-template の ADR 0013](../../../template/docs/adr/0013-devcontainer-off-the-control-plane-network.md)）。依頼は Slack と、
ホストの `acp run` から行う。

- devcontainer には API のトークン（`ACP_API_TOKEN`）を渡さない。渡すと、そのコンテナから GitHub の認証情報を持つ
  エージェントを起動できてしまう。control-plane を試すときに、人がその場で渡すのは除く。

<a id="protection"></a>

## 既定のブランチの守り

ターゲットのリポジトリは、既定のブランチをルールセットで守る（[app ADR 0025](../../../docs/adr/app/0025-pr-review-as-the-last-line.md)、
[app ADR 0027](../../../docs/adr/app/0027-require-human-approval-in-ruleset.md)）。エージェントの認証情報だけでは既定の
ブランチに入れないことを、ロールと control-plane は前提にする。守りはリポジトリの管理者が用意する（手順は
target-template の[既定のブランチの守りの用意](../../../template/docs/guides/github.md "在りか")）。

- 既定のブランチは消せず、force push できない。
- 既定のブランチへの変更は PR 経由に限り、必須のチェック `check` を求める。`check` を報告する元は GitHub Actions に
  限る。
- PR に承認を 1 件求め、承認する主体を人に限る。個人のリポジトリでは `.github/CODEOWNERS` に人だけを書いて
  コードオーナーの承認を求め、organization のリポジトリでは人だけの team を必須のレビュアーにする。エージェントの
  GitHub App は、CODEOWNERS の持ち主にも team にも入れない。
- 新しいコミットが push されたら承認を取り消す。バイパスは、リポジトリの管理者（organization の
  リポジトリでは organization の管理者も）に PR のときだけ許す。

## CLAUDE.md

エージェントに常に読み込まれる入口（[target-template の ADR 0012](../../../template/docs/adr/0012-claude-md-sections.md)）。次の節をこの順に持ち、どの節も
短くして、詳しいことは文書へのリンクにする。当てはまらない節は「なし」と書いて残す。

| 節 | 書くこと |
| --- | --- |
| 冒頭（見出しの下） | 何のためのリポジトリか（1〜2 文） |
| 使われ方 | 誰が、どこで、どう使うか |
| 公開と運用 | どこに、どう公開・配布しているか（本番の環境、デプロイの仕方）。手元の動かし方は文書へのリンク |
| 守る互換 | 壊してはいけないもの（公開した URL やデータの形式、API など）と、破壊的に変えてよい範囲 |
| 参照先 | 索引（`docs/README.md`）と全体の解説（`docs/overview.md`、あれば）へのリンクと、それぞれ何があるか |
| 作業の完了条件 | `make check` など、変更後に通すもの |
| 言語 | 依頼への回答、ドキュメント、コミットメッセージ、コード中のコメントの言語 |
| 共通のルール | conventions プラグインが読み込ませること（文面は下のとおり） |

共通のルールの節の文面：

```markdown
## 共通のルール

コメントとドキュメントの書き方など、どのターゲットでも同じルールは、`.claude/settings.json` で有効にしている
conventions プラグイン（acp-platform マーケットプレイスで配る）が、セッションとサブエージェントの始めに読み込ませる。
```

ほかに、そのターゲットだけの決まり（ほかのリポジトリの扱いなど）は、この後に節を足す。
