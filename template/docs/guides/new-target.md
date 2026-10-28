# 新しいターゲットを作る

target-template のファイルから、新しいターゲットのリポジトリを作り、control-plane から動かせるようにするまでの手順。
何をどの順で行い、どの文書に沿うかだけをここに書く。ターゲットが用意するものは
[ターゲットの契約](../../../platform/docs/reference/target.md "在りか")に、ファイルごとの共通の範囲と埋める部分は
[ファイルの分類](../overview.md#files "在りか")にある。

## 1. ファイルを写す

[ファイルの分類](../overview.md#files "在りか")の共通と埋めるファイルを、agent-control-plane のリポジトリのルートから、新しい
リポジトリの同じパスに写す。ルートのファイルは agent-control-plane が自分のために埋めた版で、共通の部分にも
そのリポジトリだけの形がある（どこかは control-plane の
[devcontainer](../../../docs/architecture/codebase/devcontainer.md "在りか") にある）。写したら、共通の部分を、それぞれの部分の
文書（[devcontainer](../architecture/devcontainer.md "在りか")、[Claude Code の設定](../architecture/claude-code.md "在りか")、
[CI](../architecture/ci.md "在りか")）の形に合わせる。

## 2. 入口と文書の根を置く

`README.md` と `docs/README.md` を置き、1 で写した `CLAUDE.md` をそのリポジトリのものに書き換える。器は conventions の
[形式：文書の根](../../../platform/plugins/conventions/docs/reference/formats.md#layout "在りか")に、`CLAUDE.md` の節は
[ターゲットの契約の CLAUDE.md](../../../platform/docs/reference/target.md#claudemd "在りか")にある。

## 3. 埋める

[ファイルの分類](../overview.md#files "在りか")の表の「ターゲットごとの部分」を、ターゲットのツールチェーンと関心に合わせて、
それぞれの部分の文書に沿って埋める。

## 4. 手元で確かめる

[devcontainer の使い方](devcontainer.md "在りか")のとおりにホストに並べ、ビルドして起動し、devcontainer の中で `make check` を
通す。

## 5. 守りを用意し、CI を確かめる

GitHub にリポジトリを作って push し、[既定のブランチの守りの用意](protection.md "在りか")のとおりに守りを用意して確かめる
（CI で確かめられないものも、その手順の「確かめる」のとおりに確かめる）。守りを用意するまで、CI の守りの検査と、それを
待つ必須のチェックは落ちる。用意したら、CI を回し直してすべて通ることを見る。

## 6. control-plane から動かす

守りを確かめてから、control-plane の設定にエージェント（ターゲットとロールの組）を足す。足し方は control-plane の
[運用の手順](../../../docs/guides/operations.md "在りか")の「サーバーを準備して起動する」にある。
