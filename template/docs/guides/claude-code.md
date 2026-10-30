# Claude Code を対話で使う

devcontainer の中で、人が Claude Code を対話で使うときの手順。設定の構成は
[architecture/claude-code.md](../architecture/claude-code.md "在りか") に、ターゲットが有効にした platform のプラグインを
入れる準備は platform の[対話で使う準備](../../../platform/docs/guides/use-interactively.md "在りか")にある。

<a id="permissions"></a>

## 権限の確認

対話の既定のモードと、確認せずに使えるツールの一覧は `.claude/settings.json` の `permissions` にある。コンテナから書ける
ホストのファイルは[マウントしたもの](../architecture/devcontainer.md#mounts)だけで、確認を省いてもその外には届かない。

```bash
claude --dangerously-skip-permissions
```

確認を省くと、次のことが確認なしで起こりうる。受け入れられるときだけ省く。

- **コンテナの中の認証情報でできること**：Claude Code 自身のログイン（`~/.claude` の下にいつもある）、`gh auth login` での
  ログイン、VS Code が attach したときに引き継ぐ git の認証情報や ssh-agent でできること（リポジトリの管理者なら、既定の
  ブランチの守りのバイパスを含む）。Claude Code のログインのほかの認証情報は、置かないか、権限を絞ったものにする。
- **外への持ち出し**：外への通信は絞っていない（[ADR 0008](../adr/0008-no-in-container-firewall.md)）。コンテナから読める
  もの（リポジトリ、隣のリポジトリ、上の認証情報）は、外に送られうる。
- **ホストで動くものの書き換え**：書き換えられると、次にホストで使ったときに動くか読まれるものがある。PR の差分に
  出ないもの（`.git/` の下の `hooks` や `config`、git が無視するファイル。マウントするホストのディレクトリを決める
  `.devcontainer/.env` もその一つ）と、差分に出てもマージの前にホストで動かしうるもの（`.devcontainer/` の
  `compose.yaml`、`init-host.sh`、`devcontainer.json`、`.vscode/` の設定）。確認を省いて使ったあとは、ホストで git、
  devcontainer、VS Code、ホストで動かすほかの道具を使う前に、それらが変わっていないことを確かめる。

<a id="copy"></a>

## 画面のコピー

Claude Code のフルスクリーン表示は、マウスで選択して離すと自動でコピーする。止めるには `/config` の Copy on select を
オフにする（設定は Claude Code の設定ディレクトリに残る）。そのあとは選択中に `Ctrl+C` か `Ctrl+Shift+C` でコピーする。
ターミナルの tmux での選択は [architecture/devcontainer.md](../architecture/devcontainer.md "在りか") にある。
