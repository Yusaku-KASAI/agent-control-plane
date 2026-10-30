# target-template

ターゲットに共通の環境のファイルと、その説明。このリポジトリがひな形を兼ね、環境のファイルはルートにある。
target-template の区切りは `target/` で、文書はその `template/docs/` にある（[ADR 0018](adr/0018-target-directory-as-the-copy-source.md)）。
ターゲットが用意するものは[ターゲットの契約](../../../platform/docs/reference/target.md "在りか")にある。文書の分け方は conventions の
[文書の体系](../../../platform/plugins/conventions/docs/architecture/documentation.md "在りか")にある。

| 文書 | 種類 | 何があるか |
| --- | --- | --- |
| [overview.md](overview.md) | 解説 | ターゲットの環境の全体の構成、ファイルの分類（共通 / 埋める / 固有） |
| [guides/new-target.md](guides/new-target.md) | 手順 | 新しいターゲットを作る（ファイルを写す、入口と文書の根を置く、埋める、確かめる、守る、control-plane から動かす） |
| [architecture/devcontainer.md](architecture/devcontainer.md) | 解説 | devcontainer の方針と構成、ターミナルと VS Code の設定 |
| [architecture/claude-code.md](architecture/claude-code.md) | 解説 | Claude Code の設定（プラグインの宣言、回答の言語、worktree の作り方、対話のときの権限） |
| [architecture/ci.md](architecture/ci.md) | 解説 | CI の構成 |
| [guides/devcontainer.md](guides/devcontainer.md) | 手順 | ホストの準備、devcontainer のビルド、起動、停止、root での作業 |
| [guides/claude-code.md](guides/claude-code.md) | 手順 | Claude Code を対話で使う（権限の確認、画面のコピー） |
| [guides/protection.md](guides/protection.md) | 手順 | 既定のブランチの守りの用意（CODEOWNERS か team、ルールセット、確かめ方） |
| [adr/](adr) | 判断 | target-template の判断 |
