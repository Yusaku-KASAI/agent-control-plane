# ターゲットの契約

ターゲットが用意するもの。control-plane と platform はこれを前提にする（[conventions ADR 0002](../../../platform/plugins/conventions/docs/adr/0002-documents-by-distribution-context.md)）。
control-plane がコンテナに渡すものは、逆向きの契約として[ハーネスの契約](../../../platform/docs/reference/harness.md "案内")にある。

<a id="image"></a>

## イメージ

control-plane は、ターゲットのイメージ（ターゲットの devcontainer のイメージ）でエージェントの
コンテナを作る。イメージは次を満たす（[ADR 0009](../adr/0009-containers-run-as-non-root.md)）。

- イメージのユーザーは root でない `node` で、ホームは `/home/node`。作業ディレクトリ（リポジトリのマウント先）に書き込める。
  管理者の権限（sudo）を持たない。
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
  - 宣言したマーケットプレイスがエージェントにどう見えるかは[ハーネスの契約](../../../platform/docs/reference/harness.md "案内")にある。
- 有効にするプラグインが前提にするプラグインも有効にする（何を前提にするかは、各プラグインの文書にある）。
- `.claude/settings.json` の `language` に、CLAUDE.md の言語の節で決めた回答の言語を書く。Claude Code はこれを毎回の
  指示として渡す。決まりは CLAUDE.md にあり、ここはそれを届ける。
- ロールは Claude Code のカスタムエージェントとして定義する（[ADR 0003](../adr/0003-roles-as-claude-code-agents.md)）。
  ターゲットの `.claude/agents/<ロール名>.md` に platform のロールと同名の定義を置くと、そちらが優先される。
  定義が見つからないロールで起動すると、エージェントはエラーで終了する。
- ロールの定義には振る舞いだけを書き、ターゲット固有の事実や規約は CLAUDE.md に書く。

<a id="checks"></a>

## 検査

- `make check` が、そのターゲットの検査をすべて行う。エージェントは変更のあとにこれを通す（CLAUDE.md の作業の完了条件）。
- `make check` は `make check-docs` を含む。`check-docs` が確かめるものは conventions の[文書の形式](../../../platform/plugins/conventions/docs/reference/formats.md "案内")の各節にある。

<a id="control-plane"></a>

## devcontainer から control-plane への接続

人が devcontainer から `acp run` で依頼できるよう、ターゲットの devcontainer は control-plane の API につながる。

- control-plane と devcontainer は、外部の Docker ネットワーク `agent-control-plane-net` を共有する。ネットワークが
  なければ、ターゲットの devcontainer のホストの準備が作る。
- devcontainer は環境変数 `ACP_SERVER=http://control-plane:8787` で control-plane の API を指す。
- devcontainer には API のトークン（`ACP_API_TOKEN`）を渡さない。渡すと、そのコンテナから GitHub の認証情報を持つ
  エージェントを起動できてしまう。

## CLAUDE.md

エージェントに常に読み込まれる入口（[ADR 0012](../adr/0012-claude-md-sections.md)）。次の節をこの順に持ち、どの節も
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
