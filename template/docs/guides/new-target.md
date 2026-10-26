# 新しいターゲットを作る

target-template のファイルから、新しいターゲットのリポジトリを作り、control-plane から動かせるようにするまでの手順。
ターゲットが用意するものは[ターゲットの契約](../../../platform/docs/reference/target.md "在りか")に、ファイルごとの共通の範囲と
埋める部分は[ファイルの分類](../overview.md#files "在りか")にある。

## 1. ファイルを写す

ひな形は agent-control-plane のリポジトリのルートにある。[ファイルの分類](../overview.md#files "在りか")の共通と埋めるファイルを、
新しいリポジトリの同じパスに写す。固有のファイル（アプリのコード、文書、ADR）は写さない。

ルートのファイルは、agent-control-plane が自分のために埋めた版で、共通の部分にも、このリポジトリだけの形が二つある
（理由は control-plane の [devcontainer](../../../docs/architecture/codebase/devcontainer.md "在りか")）。写したあと、ひな形の形に戻す。

- `.github/workflows/ci.yml` の `check-docs` と `check-protection` のジョブは、platform のアクションをリポジトリの中の
  パス（`./platform/...`）で呼んでいる。ターゲットでは `@main` で呼ぶ（[CI](../architecture/ci.md "在りか")）。
- `Makefile` の `check` は `check-docs` を含んでいる。ターゲットでは含めない（同じく CI）。

## 2. 埋める

[ファイルの分類](../overview.md#files "在りか")の表の「ターゲットごとの部分」を、ターゲットのツールチェーンと関心に合わせて
埋める。

- `.devcontainer/Dockerfile` のツールチェーンのステージと `check-<関心>` のステージ、`Makefile` の `check-<関心>`、
  `ci.yml` の行列のジョブを、関心ごとに同じ名前でそろえる（[CI](../architecture/ci.md "在りか")）。
- `.claude/settings.json` に、使うプラグインと回答の言語を書く（[Claude Code の設定](../architecture/claude-code.md "在りか")）。
- `CLAUDE.md` は、ターゲットの契約の節をこの順に持たせて書く。
- 個人のリポジトリなら `.github/CODEOWNERS` に持ち主を書き、organization のリポジトリなら消す
  （[既定のブランチの守りの用意](protection.md "在りか")）。

## 3. 動かして確かめる

1. [devcontainer の使い方](devcontainer.md "在りか")のとおりにホストに並べ、ビルドして起動する。
2. devcontainer の中で `make check` を通す。
3. GitHub にリポジトリを作って push し、CI が通ることを見る。

## 4. 守りを用意する

[既定のブランチの守りの用意](protection.md "在りか")のとおりに、承認する人とルールセットを用意し、CI の `check-protection`
が通ることを見る。

## 5. control-plane から動かす

control-plane の設定にエージェント（ターゲットとロールの組）を足す。足し方は control-plane の
[運用の手順](../../../docs/guides/operations.md "在りか")の「サーバーを準備して起動する」にある。
