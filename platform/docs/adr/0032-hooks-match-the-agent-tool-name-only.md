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

- Claude Code が別名で hook を呼ぶ版に変わると、hook が働かなくなる。版を上げるときは、サブエージェントの呼び出しに
  hook が働くことを確かめる。
- 確かめたのは Claude Code 2.1.292 の本体を読んだことによる。hook の入力を実際に記録しては確かめていない。
