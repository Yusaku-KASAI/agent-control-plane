# Claude Code を対話で使う

devcontainer の中で、人が Claude Code を対話で使うときの手順。設定の構成は
[architecture/claude-code.md](../architecture/claude-code.md "在りか") にある。

<a id="permissions"></a>

## 権限の確認

対話の既定のモードと、確認せずに使えるツールの一覧は `.claude/settings.json` の `permissions` にある。ホストのファイルは
コンテナで隔離しているので、確認を省いて使ってもよい。

```bash
claude --dangerously-skip-permissions
```

ただし、コンテナの中で `gh auth login` などでログインしているときは、その認証情報でできること（リポジトリの管理者なら、
既定のブランチの守りのバイパスを含む）が確認なしで行われうる。確認を省くのは、ログインしていないときか、権限を絞った
認証情報を使うときにする。

<a id="copy"></a>

## 画面のコピー

Claude Code のフルスクリーン表示は、マウスで選択して離すと自動でコピーする。止めるには `/config` の Copy on select を
オフにする（設定は Claude Code の設定ディレクトリに残る）。そのあとは選択中に `Ctrl+C` か `Ctrl+Shift+C` でコピーする。
ターミナルの tmux での選択は [architecture/devcontainer.md](../architecture/devcontainer.md "在りか") にある。
