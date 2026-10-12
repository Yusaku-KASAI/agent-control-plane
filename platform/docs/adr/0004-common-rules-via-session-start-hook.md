---
status: accepted
date: 2026-10-07
amended_by: ["../../plugins/conventions/docs/adr/0005-principles-and-conventions-plugins.md"]
---

# 0004. どのターゲットでも同じルールは、rules プラグインがセッションとサブエージェントの始めに読み込ませる

## Context

コメントとドキュメントのルールなど、どのターゲットでも同じにしたいルールが、このリポジトリの CLAUDE.md にしか
なかった。2つ目のターゲットでも同じルールで作業させたい。control-plane から起動するエージェントは headless の
`claude -p` で動く。試したところ、CLAUDE.md の `@パス` による取り込みは、リポジトリの中のファイルなら headless でも
読み込まれるが、リポジトリの外のファイルは対話で承認するまで読み込まれない。プラグインの SessionStart の hook が
返す `additionalContext` は、headless でも stream-json の入出力でも読み込まれる。ただしサブエージェントには
届かず（CLAUDE.md は届く）、SubagentStart の hook の `additionalContext` なら届く。

## Decision

- どのターゲットでも同じルールは、platform の rules プラグインの `rules/*.md` に置く。
- rules プラグインは、SessionStart と SubagentStart の hook で `rules/*.md` をつなげて読み込ませる。SessionStart は
  起動、再開、`/clear`、会話の要約（compact）のたびに動くので、要約したあともルールは残る。ターゲットは他の
  プラグインと同じく `.claude/settings.json` で有効にする。
- ターゲットごとに決めること（言語、完了条件、アプリの仕様を破壊的に変えてよいかなど）はターゲットの CLAUDE.md に
  書き、食い違うときは CLAUDE.md に従う。

## Alternatives

- **ターゲットの CLAUDE.md から `@/workspace/agent-platform/...` で取り込む**：headless では、リポジトリの外の
  取り込みが承認待ちで読み込まれない。
- **target-template の CLAUDE.md にルールを書いて、ターゲットにコピーする**：ターゲットの間でずれていき、
  直しても行き渡らない。
- **スキルにする**：スキルは必要と判断されたときだけ読み込まれる。ルールはいつも守らせたい。

## Consequences

- ルールを変えると、次にセッションのプロセスが起動したときから、そのマーケットプレイスを使うすべてのターゲットに届く。
- ターゲットの CLAUDE.md には、ターゲットごとのことと、共通のルールの在りかだけが残る。
- ルールは会話の中に足される文脈で、CLAUDE.md のような「プロジェクトの指示」としては扱われない。効き目が
  足りなければ、仕組み（hook や検査）で確かめる形に寄せる。
- 再開するたびに同じルールが会話に足され、会話の中で重複する。
