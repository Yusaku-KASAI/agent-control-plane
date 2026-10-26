# Claude Code を対話で使う

devcontainer の中で、人が Claude Code を対話で使うときの手順。設定の構成は
[architecture/claude-code.md](../architecture/claude-code.md "在りか") にある。

<a id="permissions"></a>

## 権限の確認

対話の既定のモードと、確認せずに使えるツールの一覧は `.claude/settings.json` の `permissions` にある。ホストのファイルのうち
コンテナから書けるのは、マウントしたリポジトリと、Claude Code と gh の状態とコマンドの履歴のディレクトリ（`compose.yaml`）だけなので、
確認を省いて使ってもよい。リポジトリの `.git/hooks` や `.git/config` を書き換えられると、ホストで git を使ったときに
動くことは残る。

```bash
claude --dangerously-skip-permissions
```

ただし、コンテナの中で使える認証情報（`gh auth login` でのログインのほか、VS Code が attach したときに引き継ぐ git の
認証情報や ssh-agent）があるときは、それでできること（リポジトリの管理者なら、既定のブランチの守りのバイパスを含む）が
確認なしで行われうる。確認を省くのは、そうした認証情報がないときか、権限を絞った認証情報を使うときにする。

<a id="copy"></a>

## 画面のコピー

Claude Code のフルスクリーン表示は、マウスで選択して離すと自動でコピーする。止めるには `/config` の Copy on select を
オフにする（設定は Claude Code の設定ディレクトリに残る）。そのあとは選択中に `Ctrl+C` か `Ctrl+Shift+C` でコピーする。
ターミナルの tmux での選択は [architecture/devcontainer.md](../architecture/devcontainer.md "在りか") にある。
