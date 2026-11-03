# 新しいターゲットを作る

target-template のファイルから、新しいターゲットのリポジトリを作り、control-plane から動かせるようにするまでの手順。
何をどの順で行い、どの文書に沿うかだけをここに書く。ターゲットが用意するものは
[ターゲットの契約](../../../../platform/docs/reference/target.md "在りか")に、ファイルごとの共通の範囲と埋める部分は
[ファイルの分類](../overview.md#files)にある。

## 1. ファイルを写す

agent-control-plane の `target/` の下を、`template/` を除いて、新しいリポジトリのルートに写す（`.` で始まるファイルと
ディレクトリも含める）。`template/` の文書は、platform と control-plane への相対リンクが写した先で切れるので写さない。

名前が `.tmpl` で終わるひな形は、写したら `.tmpl` を外す（[ファイルの分類](../overview.md#files)）。

```bash
cp -R <agent-control-plane>/target/. <新しいリポジトリ>/ && rm -r <新しいリポジトリ>/template
find <新しいリポジトリ> -name '*.tmpl' -exec sh -c 'mv "$1" "${1%.tmpl}"' _ {} \;
```

## 2. 入口と文書の根を置く

1 で名前を戻した `README.md`、`docs/README.md`、`docs/overview.md`、`CLAUDE.md` の `Fill in:` の所と `<リポジトリ名>` を、
そのリポジトリのものに書く。器は conventions の
[形式：文書の根](../../../../platform/plugins/conventions/docs/reference/formats.md#layout "在りか")に、`CLAUDE.md` の節は
[ターゲットの契約の CLAUDE.md](../../../../platform/docs/reference/target.md#claudemd "在りか")にある。

## 3. 埋める

[ファイルの分類](../overview.md#files)の表の「ターゲットごとの部分」（ひな形では `Fill in:` のコメントの所と、コメントを書けない `.claude/settings.json` の部分）を、
ターゲットのツールチェーンと関心に合わせて、それぞれの部分の文書（[devcontainer](../architecture/devcontainer.md)、
[Claude Code の設定](../architecture/claude-code.md)、[CI](../architecture/ci.md)）に沿って埋める。`.devcontainer/.env.example` の
`<repository>` と `.github/CODEOWNERS` の `<owner>` も埋める。

## 4. 手元で確かめる

[devcontainer の使い方](devcontainer.md)のとおりにホストに並べ、ビルドして起動し、devcontainer の中で `make check` を
通す。

## 5. 守りを用意し、CI を確かめる

GitHub にリポジトリを作って push し、[既定のブランチの守りの用意](protection.md)のとおりに守りを用意して、その手順の
「[確かめる](protection.md#verify)」のとおりに CI と守りを確かめる。

## 6. control-plane から動かす

守りを確かめてから、control-plane の設定にエージェント（ターゲットとロールの組）を足す。足し方は control-plane の
[運用の手順](../../../../docs/guides/operations.md "在りか")の「サーバーを準備して起動する」にある。
