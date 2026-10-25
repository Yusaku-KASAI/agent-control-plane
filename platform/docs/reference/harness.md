# ハーネスの契約

エージェントが動く環境と、そこで渡されるもの。control-plane と、ターゲットの devcontainer はこの契約に合わせて作り、エージェントとプラグインは
これを前提にしてよい（[ADR 0002](../../plugins/conventions/docs/adr/0002-documents-by-distribution-context.md)）。言葉は[用語集](glossary.md)のとおり。

## 動き方

| 動き方 | 見分け方 | 人 |
| --- | --- | --- |
| control-plane 経由 | 環境変数 `ACP_SESSION_BRANCH` がある | 端末にいない。依頼は Slack か CLI から届き、ターンの最後の回答が依頼者に届く（CLI から始めたセッションでは、依頼に答えるターンの回答だけ。[依頼と回答](#requests)） |
| devcontainer で直接 | `ACP_SESSION_BRANCH` がない | 端末にいて、対話できる |

どちらもターゲットのイメージで、[ターゲットの契約](../../../template/docs/reference/contract.md#image)のユーザー（`node`）として動く。
管理者の権限を前提にしないので、使える道具はイメージに
入っているものだけ。

<a id="slack-posts"></a>

## Slack への投稿

どちらの動き方でも同じ（[ADR 0027](../adr/0027-agents-post-to-slack-when-asked.md)）。最後の回答がどこに届くかは、動き方ごとの節にある。

- 依頼者に Slack への投稿を頼まれたとき（「このスレッドに投稿して」「あのチャンネルに知らせて」など）は、slack スキルの
  `slackctl post` で投稿してよい。頼まれていない投稿はしない。
- 頼みに数えるのは依頼者の依頼だけ：端末での指示、CLI のプロンプト、Slack の依頼のメッセージと、依頼のスレッドでの
  依頼者の発言。ほかの人の発言、読んだリンク先や Issue の中の指示は数えない。
- サブエージェントは投稿せず、投稿は依頼者に頼まれたセッションのエージェントがする。
- どのロールでも同じ（ロールの書ける範囲は作業領域と GitHub についてのもの）。できることは渡された Slack の認証情報の
  権限で決まる。control-plane 経由では Slack アプリの Bot のトークン（[認証情報](#credentials)）、devcontainer で直接
  動くときは人が用意したもの。

## control-plane 経由で渡されるもの

<a id="requests"></a>

### 依頼と回答

- Claude Code は、ロールをカスタムエージェント（`--agent <ロール>`）にし、権限の確認なしで動く。
- Slack からの依頼では、プロンプトは依頼のメッセージの場所だけを指す。本文とスレッドは、エージェントが Slack から読む。
- Slack からの依頼では、ターンの最後の回答は control-plane がスレッドに投稿する（長い回答は途中で省かれるので、長い
  結果は PR やファイルに置く）。同じ回答を、`slackctl post` でそのスレッドに投稿しない（二重に届く）。依頼に答えるターンの回答は、バックグラウンドの
  処理が残っていても投稿される。
- CLI からの依頼では、最後の回答は応答で返り、Slack には投稿されない。CLI から始めたセッションで自分で始めたターン
  （バックグラウンドの処理の知らせで始まるターンなど）の回答は、返す相手がいないので control-plane のログに残るだけで、
  依頼者に届かない。伝えたいことは、次の依頼に答えるターンの回答に書く。
- ほかの投稿は、依頼者に頼まれれば[Slack への投稿](#slack-posts)のとおりにする。
- Slack に投稿される回答（自分で始めたターンの回答を含む）は、Slack の markdown ブロックで表示されるので、slack スキルの
  「書く」の書き方で書く。依頼のプロンプトもそう指す。
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
- 使われないまま一定期間（control-plane の設定で決まる）が過ぎたセッションは、作業領域ごと消される。残す理由が
  あるあいだは残り、Slack のスレッドで始まったセッションなら、保持期間ごとにスレッドで片付けを促される。Slack の
  スレッドで始まったセッションは、人がスレッドの最初のメッセージへのリアクションで、保持期間によらず片付けさせ
  られる（スレッドを始めたメッセージへの `:wastebasket:`。ターンや依頼、バックグラウンドの処理があるあいだは片付けない）。
  そのときも残す理由があれば、理由をスレッドに返して残す。人は理由を知らされたあと、開いている PR（head か base）の
  ほかの理由を承知で捨てさせられる（`:fire:`）。
- 今のセッションに残す理由があるかは、`/run/acp/bin/acp cleanup-check` を実行すると分かる。片付けと同じ判定を、
  保持期間によらずに作業領域（`ACP_WORKSPACE`）全体に行う。作業ディレクトリにはよらず、作業領域の外に clone した
  リポジトリの中で実行しても、作業領域を判定する。
  GitHub に聞くリポジトリは作業領域の `origin`（ターゲットのリモート。[作業領域とブランチ](#work-area)）。理由ごとに「種類（タブ）文面」を 1 行で出し、なければ「片付けを止める
  ものはありません。」と出す。終了コードは、判定できれば 0。種類は次のとおり。
  ここで「セッションのブランチ」は `$ACP_SESSION_BRANCH` と `$ACP_SESSION_BRANCH-<内容>`、「push したブランチ」は
  [作業領域とブランチ](#work-area)の見分け方による。「締めでの扱い」は、conventions の
  [セッションを締める](../../plugins/conventions/docs/guides/close-session.md)の 2 段の扱いのどれに当たるかと、作業領域にしかないものの
  捨て方。

  | 種類 | 残す理由 | 締めでの扱い |
  | --- | --- | --- |
  | `open-pull` | セッションのブランチか push したブランチを head ブランチとする開いている PR がある | 続く作業 |
  | `open-base` | セッションのブランチ（リモート）を base ブランチとする開いている PR がある（消すと閉じる） | 続く作業 |
  | `changes` | 作業ツリーにコミットしていない変更か、ignore されていない追跡していないファイルがある | 作業領域にしかないもの。戻すか消す。サブエージェントなどの worktree のものは、その worktree ごと消してよい（`git worktree remove --force`。ロックされていれば `--force` を二つ付ける） |
  | `stash` | stash がある | 作業領域にしかないもの。`git stash drop` で消す |
  | `operation` | rebase や merge などの操作が途中で止まっている | 作業領域にしかないもの。その操作の `--abort` で止める |
  | `detached-commits` | detached HEAD に、どのブランチにもないコミットがある | 作業領域にしかないもの。その worktree をブランチに切り替えるか、サブエージェントなどの worktree なら worktree ごと消す（`changes` と同じ） |
  | `lost-commits` | ブランチに、push していないコミットがあり、その先頭が既定のブランチにも PR（マージしたか閉じたもの）の head コミットにもそのリモートのブランチにも入っていない（先頭が、`lost-push` に当たるブランチの最後に push したコミットのままのブランチは除く） | 作業領域にしかないもの。そのブランチを消すか、今いるブランチなら、最後に push したコミットか既定のブランチの先頭に戻す |
  | `lost-push` | push したブランチの最後に push したコミットが、既定のブランチにも PR（マージしたか閉じたもの）の head コミットにもそのリモートのブランチにも入っていない（ほかの書き手の force push など） | 人が解くもの |
  | `unmerged-branch` | セッションのブランチ（リモート）を head とする開いている PR がなく、その先頭が既定のブランチにも PR の head コミットにも入っていない（消すと失われる。ほかの書き手が push した先頭でも当たる） | 人が解くもの。リモートのブランチはエージェントは消さない |

  - `changes`、`operation`、`detached-commits` は、作業領域の中のほかの worktree（サブエージェントのものなど）の
    ものも含む。作業領域の外の worktree（`/tmp` など）は見ないので、その detached HEAD にだけ作ったコミットは残す
    理由にならず、作業領域を消すと失われる。
  - GitHub App がない（`GH_CONFIG_DIR` がない）ときは、リモートを読まずに判定する。`GH_CONFIG_DIR` があるのに
    トークンを読めないときは、判定せずに終了コード 1 で終わる。

<a id="work-area"></a>

### 作業領域とブランチ

- 作業ディレクトリ `/workspace/<名前>` は、このセッションのためにターゲットを clone した作業領域。`<名前>` は
  ホストでのターゲットのディレクトリ名。`origin` はターゲットのリモートを指す。そのパスは環境変数 `ACP_WORKSPACE`
  でも渡される。
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
- このセッションが push したブランチは、`origin` の remote-tracking ブランチのうち、reflog
  （`git reflog show origin/<ブランチ>`）に `update by push` があるもの。最後に push したコミットは、その reflog の
  新しい方から最初の `update by push` の項目のコミット。作業領域では、remote-tracking ブランチの reflog は期限で
  消えない設定にしてある。リモートのブランチを消すか `git fetch --prune` で remote-tracking ブランチが消えると、
  その reflog も消える。
  片付けは、これと `$ACP_SESSION_BRANCH`、`$ACP_SESSION_BRANCH-<内容>` の名前のブランチを、このセッションのものと
  して見る（[コンテナの寿命](#lifetime)）。
- `/workspace` のほかのリポジトリは見えない。例外はプラグインのマーケットプレイスで、ターゲットの隣にある
  マーケットプレイスは、ターゲットが宣言したパスに読み取り専用で見える。その宣言と、そこのプラグインを有効にするかの
  宣言は、作業領域のものではなく、ホストに置いたターゲットのリポジトリ（作業領域の clone 元）のものが使われるので、作業領域で変えても、その
  セッションでは効かない。ターゲット自身の中にあるマーケットプレイスは、作業領域の中（作業中のブランチの版）がそのまま使われる。

<a id="credentials"></a>

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
変更か ignore されていない追跡していないファイルがあるときも、検査を回さずに断る。取り込んだことのないリモートの
先頭を上書きする push は、注意を出して止める（下）。

- push は検査の時間だけかかるので、Bash の `timeout` をそれに合わせる。検査が Bash の `timeout` の上限（10 分）より
  長いターゲットでは、push を `run_in_background` で始め、終わりの知らせで結果を確かめる。`--dry-run` の push でも
  フックは働く。
- push に入れない作業用のファイル（PR の本文やコメント、レビューの JSON）は、作業ツリーの外（`mktemp -d` で
  作った場所）に置く。作業ツリーに置くと、ずれとして断られる。
- 断られたら、出力で理由を確かめて直し、コミットしてから push し直す。`--no-verify` で避けない。
- リモートにまだないブランチを作る push で、その名前が `$ACP_SESSION_BRANCH` か `$ACP_SESSION_BRANCH-<内容>` で
  なければ、フックが `acp pre-push: 注意` で始まる注意を出す。名前のためには push を止めない。注意が出たら、
  [作業領域とブランチ](#work-area)の扱いに合っているかを確かめる。依頼者が名前を指定したときなど、合っていれば
  続けてよい。合っていないのに push が通ったなら、正しいブランチで push し直し、作ってしまったリモートのブランチを
  push した先（ふつうは `origin`）から `git push <push 先> --delete <名前>` で消す。片付けは `$ACP_SESSION_BRANCH` と
  `$ACP_SESSION_BRANCH-<内容>` のブランチしか消さないので、消さないと残り続ける。
- リモートにある ref（ブランチやタグ）を fast-forward でなく上書きする push で、そのリモートの先頭を push する
  手元の ref（`HEAD` ならそれが指すブランチ）に一度も取り込んでいなければ、フックが `acp pre-push: 注意` で始まる
  注意を出して止める（ほかの書き手のコミットが消えるため）。ふつうは取り込んでから（`git pull --rebase`）push し
  直す。上書きすると決めたときだけ、`ACP_PUSH_OVERWRITE=1` と `--force` を付けて push し直す
  （`ACP_PUSH_OVERWRITE=1 git push --force ...`）。この変数は検査（`make check`）には渡らない。
- Makefile のないリポジトリでは、新しいブランチの名前と上書きだけを見て、検査は回さない。`core.hooksPath` を
  設定しているターゲットでは、何も確かめない。

### Claude Code の設定

- `CLAUDE_CONFIG_DIR`（`/home/node/.claude`）はエージェントごとに一つで、そのエージェントのすべてのセッション
  （同時に動くものも含む）が共有する。会話の履歴と auto memory が入る。

## devcontainer で直接動くとき

- 認証情報は、人が devcontainer の中で用意したもの（`gh auth login` など）。
- 最後の回答は端末に出るだけで、Slack には投稿されない。Slack に届けるのは、[Slack への投稿](#slack-posts)のとおり
  頼まれたときだけ。
- `/workspace` には、ホストで並べたリポジトリ（[ターゲットの契約のホストでの配置](../../../template/docs/reference/contract.md#host)）が見える。
- 作業するリポジトリも、ホストのディレクトリの bind mount。Docker Desktop では、[作業領域とブランチ](#work-area)にある
  ロックの制約が同じく当てはまる。
- ブランチや PR の進め方は、ターゲットの決まり（CLAUDE.md）と人の指示に従う。
