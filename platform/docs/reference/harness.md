# ハーネスの契約

エージェントが動く環境と、そこで渡されるもの。control-plane と、ターゲットの devcontainer はこの契約に合わせて作り、エージェントとプラグインは
これを前提にしてよい（[ADR 0002](../../plugins/conventions/docs/adr/0002-documents-by-distribution-context.md)）。言葉は[用語集](glossary.md)のとおり。

## 動き方

| 動き方 | 見分け方 | 人 |
| --- | --- | --- |
| control-plane 経由 | 環境変数 `ACP_SESSION_BRANCH` がある | 端末にいない。依頼は Slack か CLI から届き、ターンの最後の回答が依頼者に届く |
| devcontainer で直接 | `ACP_SESSION_BRANCH` がない | 端末にいて、対話できる |

どちらもターゲットのイメージで、[ターゲットの契約](../../../template/docs/reference/contract.md#image)のユーザー（`node`）として動く。
管理者の権限を前提にしないので、使える道具はイメージに
入っているものだけ。

## control-plane 経由で渡されるもの

### 依頼と回答

- Claude Code は、ロールをカスタムエージェント（`--agent <ロール>`）にし、権限の確認なしで動く。
- Slack からの依頼では、プロンプトは依頼のメッセージの場所だけを指す。本文とスレッドは、エージェントが Slack から読む。
- ターンの最後の回答は control-plane がスレッドに投稿する（長い回答は途中で省かれるので、長い結果は PR やファイルに
  置く）。自分で投稿しない。依頼に答えるターンの回答は、バックグラウンドの処理が残っていても投稿される。
- サブエージェント（Agent）は、harness プラグインの hook がフォアグラウンドで動かす
  （[ADR 0008](../adr/0008-subagents-in-foreground-under-control-plane.md)）。hook は、Agent の呼び出しの `run_in_background` を
  `false` にし、必ずバックグラウンドで動く `isolation: remote` の Agent と `SendMessage`（宛先によらない）を断る。
  サブエージェントの続きは Agent で新しく頼む。エージェントの定義の frontmatter に `background: true` か
  `isolation: remote` があると、hook によらずバックグラウンドで動くので、そういう定義は使わない。
- サブエージェントが動いている間に作業領域（主の worktree）が変わると、harness プラグインの hook が、変わったことを
  セッションのエージェントに知らせる（比べるものは [ADR 0016](../adr/0016-subagents-leave-the-work-area.md)）。
  作業を任せたサブエージェントの変更でも知らせるので、コミットの前に git で何が変わったかを確かめる。どのロールが自分の worktree で
  動くかは roles の定義にある。
- バックグラウンドの処理が終わると、知らせが届く（動いているターンに入るか、自分で始めるターンになる）。終わりを
  待つために、Monitor、ScheduleWakeup、`sleep` の繰り返し、同じ処理のフォアグラウンドでの実行で見張らない。見張りも
  ターンやバックグラウンドの処理を増やし、そのぶん回答が増える（[ADR 0010](../adr/0010-one-ci-wait-per-branch.md)）。
- 処理をバックグラウンドで動かすときは、Bash の `run_in_background` を使う。`&`、`nohup`、`disown`、`setsid` で
  手放した処理は、終わりの知らせが届かず、control-plane のバックグラウンドの処理にも数えられない。harness
  プラグインの hook が、手放す Bash（`&` のあとに同じコマンドの中で `wait` するものは除く）と、Monitor、
  ScheduleWakeup、CronCreate（待つための予約）を断る（[ADR 0015](../adr/0015-refuse-let-go-processes-under-control-plane.md)）。
- バックグラウンドの処理が終わって自分で始めたターンの回答は、ターンの終わりにほかのバックグラウンドの処理が
  残っていれば、すぐには投稿されない。投稿されるのは次の場合。
  - 次のターンが始まらずに処理が尽きたとき。
  - 次のターンがエラー（使用上限を含む）やプロセスの失敗で終わったとき（流し込んだ依頼が入ったターンでも）。
  - 次のターンが回答を出さずに終わって、処理が尽きたとき。

  次のターンの回答や、依頼に答えるターン（`:zap:` で流し込んだ依頼を含む）は、まだ投稿されていない前の回答を
  置き換える。届いた依頼を取り込む前に自分で終えたターン（再開したときに、前のプロセスで終わらなかった処理の
  知らせで始まるターンなど）の回答も、依頼に答えるターンが置き換える。ただし、その依頼が途中に流し込まれた
  ターンは、上のとおりに扱われる。伝えたいことは、それまでの分もまとめて書く。
- ターンの途中で依頼者に質問して返事を待つことはできない。同じスレッドへの返信は同じセッションの次のターンになる
  ので、問いかけで終えることはできる。
- ターンの途中にも、プロンプトが届くことがある（Slack の `:zap:` で始まる発言、CLI の `--inject`）。待っていた
  依頼がまとめて届くこともある。最後の回答は、届いたすべてに答える。

<a id="lifetime"></a>

### コンテナの寿命

- コンテナはターンをまたいで動き続ける。依頼もバックグラウンドの処理もない状態が続くと、コンテナは**消され**、
  次の依頼で新しいコンテナで会話を再開する。control-plane の再起動でも、動いていた処理は中断され、コンテナは消される。
- 残るのは作業領域と `CLAUDE_CONFIG_DIR` だけ。`/tmp`、キャッシュ、自分で入れたもの、バックグラウンドの
  プロセスは残らない。
- コンテナのルートと `/tmp`（とキャッシュ）は、コンテナの書き込み層（`overlay`）にある。Docker Desktop では VM の
  ディスクに載り、`df` で見える空きは作業領域（ホストのディレクトリ）とは別になる。VM のディスク自体もホストの
  ディスクの上にあるので、ホストが満ちると、`df` が空きを示していても書き込めないことがある。
- 使われないまま一定期間（control-plane の設定で決まる）が過ぎたセッションは、作業領域ごと消される。push していないコミット、
  作業ツリーの変更、開いている PR があるあいだは残る。

<a id="work-area"></a>

### 作業領域とブランチ

- 作業ディレクトリ `/workspace/<名前>` は、このセッションのためにターゲットを clone した作業領域。`<名前>` は
  ホストでのターゲットのディレクトリ名。`origin` はターゲットのリモートを指す。
- 作業領域は、ホストのディレクトリをコンテナに bind mount したもの。Docker Desktop（fakeowner のマウント）では、
  その上のファイルのロック（`flock`）が取り合いにならない（二つ目も取れる）。プロセスの間で取り合うロックは、
  コンテナの `TMPDIR`（なければ `/tmp`）に置く。そこで取り合いになるのは、同じコンテナの中のプロセスの間だけ。
  `TMPDIR` の中身（ロックを含む）は、コンテナが作り直されると消える（[コンテナの寿命](#lifetime)）。
- セッションのブランチ `ACP_SESSION_BRANCH` は、GitHub App があり、ターゲットに `origin` があればリモートの既定の
  ブランチから、そうでなければターゲットの今の HEAD から切られている。別の PR に分けるときは、リモートの既定のブランチから
  `$ACP_SESSION_BRANCH-<内容>` を切る。ほかの PR に重ねるとき（重ねてよいときは conventions の変更の扱い方）は、
  その PR のブランチから切る。
- 既存の PR を続けるとき（ほかのセッションや人が作った PR も含む）は、新しいブランチを切らない。その PR の head
  ブランチを取ってきて作業し、push してその PR を更新する（[ADR 0020](../adr/0020-continue-a-pr-on-its-head-branch.md)）。
  取ってくるときは `git fetch origin <ブランチ>` のあと `git switch <ブランチ>` する。手元に同じ名前のブランチが
  もとからあったときは、そのブランチに switch したあと、続けて `git pull --rebase origin <ブランチ>` でリモートの
  先頭に合わせる。リモートが rebase と force push で書き換えられていても、書き換え前に push したコミットは除かれ、
  まだ push していない自分のコミットだけが載る。
- `/workspace` のほかのリポジトリは見えない。例外はプラグインのマーケットプレイスで、ターゲットの隣にある
  マーケットプレイスは、ターゲットが宣言したパスに読み取り専用で見える。ターゲット自身の中にあるマーケットプレイス
  は、作業領域の中（作業中のブランチの版）がそのまま使われる。

### 認証情報

GitHub App や Slack アプリはエージェントごとに任意なので、ないものの環境変数は渡されない。

| 環境変数 | 渡されるとき | 中身 |
| --- | --- | --- |
| `GH_CONFIG_DIR` | GitHub App がある | gh の設定（`/run/acp/credentials/gh`）。GitHub App のトークンが入る。git も github.com には gh を通して HTTPS で認証する（SSH の URL も HTTPS に読み替える） |
| `SLACK_BOT_TOKEN_FILE` | Slack アプリがある | Slack の Bot のトークンのファイル（`/run/acp/credentials/slack/bot-token`） |
| `CLAUDE_CODE_OAUTH_TOKEN` か `ANTHROPIC_API_KEY` | いつも | Claude Code の認証。Claude Code 自身が使い、エージェントが実行するコマンドの環境からは見えない |

- `/run/acp/credentials` は読み取り専用。トークンは control-plane が失効する前に書き換えるので、毎回ファイルから
  読む。`gh auth login` や `gh config set` のように設定を書き換えるコマンドは失敗する。
- できることは、エージェントの GitHub App と Slack アプリの権限で決まる。

### コミット

| 環境変数 | 渡されるとき | 中身 |
| --- | --- | --- |
| `GIT_AUTHOR_NAME`、`GIT_AUTHOR_EMAIL`、`GIT_COMMITTER_NAME`、`GIT_COMMITTER_EMAIL` | エージェントのコミットの作者が設定されている | エージェントの名前（GitHub App の Bot） |
| `ACP_CO_AUTHORS_FILE` | いつも | 今のターンの依頼者を 1 行に 1 人（`名前 <メール>`）で書いた一覧（`/run/acp/credentials/git/co-authors`） |

作業領域の `.git/hooks/commit-msg` が、一覧の人を `Co-authored-by` としてコミットに足す。ターゲットが
`core.hooksPath` を設定しているときや、`--no-verify` でコミットしたときは足されない。

<a id="push"></a>

### push

作業領域の `.git/hooks/pre-push` が、push の前に[ターゲットの検査](../../../template/docs/reference/contract.md#checks)（`make check`）を
回し、落ちれば push を断る。push する ref（消すものは除く）が今の HEAD でないときと、作業ツリーにコミットしていない
変更か ignore されていない追跡していないファイルがあるときも、検査を回さずに断る。

- push は検査の時間だけかかるので、Bash の `timeout` をそれに合わせる。検査が Bash の `timeout` の上限（10 分）より
  長いターゲットでは、push を `run_in_background` で始め、終わりの知らせで結果を確かめる。`--dry-run` の push でも
  フックは働く。
- push に入れない作業用のファイル（PR の本文やコメント、レビューの JSON）は、作業ツリーの外（`mktemp -d` で
  作った場所）に置く。作業ツリーに置くと、ずれとして断られる。
- 断られたら、出力で理由を確かめて直し、コミットしてから push し直す。`--no-verify` で避けない。
- リモートにまだないブランチを作る push で、その名前が `$ACP_SESSION_BRANCH` か `$ACP_SESSION_BRANCH-<内容>` で
  なければ、フックが `acp pre-push: 注意` で始まる注意を出す。push は止めない。注意が出たら、[作業領域とブランチ](#work-area)の
  扱いに合っているかを確かめる。依頼者が名前を指定したときなど、合っていれば続けてよい。
- Makefile のないリポジトリでは、新しいブランチの名前だけを確かめ、検査は回さない。`core.hooksPath` を設定している
  ターゲットでは、何も確かめない。

### Claude Code の設定

- `CLAUDE_CONFIG_DIR`（`/home/node/.claude`）はエージェントごとに一つで、そのエージェントのすべてのセッション
  （同時に動くものも含む）が共有する。会話の履歴と auto memory が入る。

## devcontainer で直接動くとき

- 認証情報は、人が devcontainer の中で用意したもの（`gh auth login` など）。
- `/workspace` には、ホストで並べたリポジトリ（[ターゲットの契約のホストでの配置](../../../template/docs/reference/contract.md#host)）が見える。
- 作業するリポジトリも、ホストのディレクトリの bind mount。Docker Desktop では、[作業領域とブランチ](#work-area)にある
  ロックの制約が同じく当てはまる。
- ブランチや PR の進め方は、ターゲットの決まり（CLAUDE.md）と人の指示に従う。
