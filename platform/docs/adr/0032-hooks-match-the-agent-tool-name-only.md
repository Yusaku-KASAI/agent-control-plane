---
status: accepted
date: 2026-10-26
amends: [8]
---

# 0032. サブエージェントの hook は、ツールの名前 `Agent` にだけ当てる

## Context

ADR 0008 は、Claude Code 2.1.292 ではサブエージェントのツールの名前が `Agent` で、`Task` はその別名なので、hook の
matcher をどちらの名前にも当てると決めた。harness の `foreground` と `watch-tree` は、`Agent|Task` で当て、スクリプトも
両方の名前を見ていた。全体のレビュー #283（判断 17）は、今の Claude Code が hook に `Task` を渡すかを確かめ、渡さなければ
外すとした。

Claude Code 2.1.292 の本体では、PreToolUse、PostToolUse、PostToolUseFailure の hook を呼ぶ所が、ツールの定義の名前（`Agent`）を `tool_name` と matcher の
照らす値に使う。別名の `Task` はツールを探すときにだけ使われ、hook には渡らない。

## Decision

- harness の hook の matcher とスクリプトは、サブエージェントのツールを `Agent` だけで見分ける。`Task` は当てない。
- Claude Code がツールの名前を変えたら、そのときに matcher を変える。

## Alternatives

- **`Task` も当てたままにする**：渡らない名前に当てても害はないが、何のために当てているかが読み手に分からず、試験も
  渡らない入力を確かめ続ける。

## Consequences

- Claude Code が hook に渡すツールの名前を変える版（別名で呼ぶ版、新しい名前にする版）では、hook が働かなくなる。
  `Task` を当てたままにしても新しい名前には当たらないので、版を上げるときに、サブエージェントの呼び出しに hook が
  働くことを確かめて拾う。
- 確かめたのは Claude Code 2.1.292 の本体を読んだことによる。hook の入力を実際に記録しては確かめていない。
- 名前が変わったことを機械で見つける手段（資格情報なしでツールの名前を出させる、名前によらず入力の形で見分ける hook）は
  今はなく、守りは、ターゲットの契約のイメージに求める版と、版を上げるときの確かめになる。
