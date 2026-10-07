# 0004. どのターゲットでも同じルールは、rules プラグインがセッションの始めに読み込ませる

- Status: Accepted
- Date: 2026-10-07

## Context

コメントとドキュメントのルールなど、どのターゲットでも同じにしたいルールが、このリポジトリの CLAUDE.md にしか
なかった。2つ目のターゲットでも同じルールで作業させたい。control-plane から起動するエージェントは headless の
`claude -p` で動く。試したところ、CLAUDE.md の `@パス` による取り込みは、リポジトリの中のファイルなら headless でも
読み込まれるが、リポジトリの外のファイルは対話で承認するまで読み込まれない。プラグインの SessionStart の hook が
返す `additionalContext` は、headless でも stream-json の入出力でも読み込まれる。

## Decision

- どのターゲットでも同じルールは、platform の rules プラグインの `rules/*.md` に置く。
- rules プラグインは、SessionStart の hook で `rules/*.md` をつなげて読み込ませる。ターゲットは他のプラグインと
  同じく `.claude/settings.json` で有効にする。
- ターゲットごとに決めること（言語、完了条件、変更の方針など）はターゲットの CLAUDE.md に書き、食い違うときは
  CLAUDE.md に従う。

## Alternatives

- **ターゲットの CLAUDE.md から `@/workspace/agent-platform/...` で取り込む**：headless では、リポジトリの外の
  取り込みが承認待ちで読み込まれない。
- **target-template の CLAUDE.md にルールを書いて、ターゲットにコピーする**：ターゲットの間でずれていき、
  直しても行き渡らない。
- **スキルにする**：スキルは必要と判断されたときだけ読み込まれる。ルールはいつも守らせたい。

## Consequences

- ルールを変えると、次にセッションのプロセスが起動したときから、そのマーケットプレイスを使うすべてのターゲットに届く。
- ターゲットの CLAUDE.md には、ターゲットごとのことと、共通のルールの在りかだけが残る。
