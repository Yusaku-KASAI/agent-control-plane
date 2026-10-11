# target-template

ターゲットに共通の環境と、ターゲットの契約。切り出すまでは、このリポジトリがひな形を兼ねる（環境のファイルは
ルートにある）。文書の分け方は platform の[文書の体系](../../platform/docs/architecture/documentation.md)にある。

| 文書 | 種類 | 何があるか |
| --- | --- | --- |
| [overview.md](overview.md) | 解説 | ターゲットの環境の全体の構成 |
| [architecture/devcontainer.md](architecture/devcontainer.md) | 解説 | devcontainer の方針と構成、Claude Code・ターミナル・VS Code の設定 |
| [architecture/ci.md](architecture/ci.md) | 解説 | CI の構成 |
| [guides/devcontainer.md](guides/devcontainer.md) | 手順 | devcontainer のビルド、起動、停止 |
| [reference/contract.md](reference/contract.md) | 参照 | ターゲットの契約（イメージ、プラグイン、検査、CLAUDE.md の節、ファイルの分類） |
| [adr/](adr) | 判断 | |
