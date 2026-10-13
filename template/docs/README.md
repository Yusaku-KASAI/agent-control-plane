# target-template

ターゲットに共通の環境のファイルと、その説明。このリポジトリがひな形を兼ね、環境のファイルはルートにある。
ターゲットの契約は、target-template ではなく契約の項目で、文書だけをここに置く。文書の分け方は conventions の
[文書の体系](../../platform/plugins/conventions/docs/architecture/documentation.md "案内")にある。

| 文書 | 種類 | 何があるか |
| --- | --- | --- |
| [overview.md](overview.md) | 解説 | ターゲットの環境の全体の構成、ファイルの分類（共通 / 埋める / 固有） |
| [architecture/devcontainer.md](architecture/devcontainer.md) | 解説 | devcontainer の方針と構成、ターミナルと VS Code の設定 |
| [architecture/claude-code.md](architecture/claude-code.md) | 解説 | Claude Code の設定（プラグインの宣言、対話のときの権限） |
| [architecture/ci.md](architecture/ci.md) | 解説 | CI の構成 |
| [guides/devcontainer.md](guides/devcontainer.md) | 手順 | ホストの準備、devcontainer のビルド、起動、停止、root での作業、Claude Code の権限の確認と画面のコピー |
| [reference/contract.md](reference/contract.md) | 参照 | ターゲットの契約（イメージ、ホストでの配置、プラグインとロール、検査、control-plane への接続、CLAUDE.md の節） |
| [adr/](adr) | 判断 | |
