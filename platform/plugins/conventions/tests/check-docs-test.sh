#!/usr/bin/env bash
# Checks that check-docs passes documents that keep each check of docs/reference/formats.md, and fails those that
# break one, with the error that names it; and which documents it reads, and which lines it reads as code.
# The backquotes are Markdown, not commands.
# shellcheck disable=SC2016
set -euo pipefail

bin=$(cd "$(dirname "$0")/.." && pwd)/bin/check-docs
failed=0
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
# Outside the repository built here, no git work tree is found, whatever holds the temporary directory.
export GIT_CEILING_DIRECTORIES=$tmp
repo=$tmp/repo

put() { mkdir -p "$(dirname "$repo/$1")"; printf '%s\n' "$2" >"$repo/$1"; }
add() { printf '%s\n' "$2" >>"$repo/$1"; }

passes() {
  local name=$1 out
  if out=$(cd "$repo" && "$bin" 2>&1); then
    echo "ok   $name"
  else
    echo "FAIL $name: it fails with '$out'"
    failed=1
  fi
}

# Run from the directory $3 inside the repository when given.
fails() {
  local name=$1 want=$2 out
  if out=$(cd "$repo/${3:-}" && "$bin" 2>&1); then
    echo "FAIL $name: it passes, want an error with '$want'"
    failed=1
  elif grep -qF -- "$want" <<<"$out"; then
    echo "ok   $name"
  else
    echo "FAIL $name: got '$out', want an error with '$want'"
    failed=1
  fi
}

# A repository whose documents keep every check: a docs root of its own, and a plugin p with its own docs root,
# a purpose and a common procedure, a role, and rules.
fresh() {
  rm -rf "$repo"
  git init --quiet -b main "$repo"
  put README.md '# r

[docs](docs/README.md)
[p](p/docs/README.md "在りか")'
  cp "$repo/README.md" "$repo/CLAUDE.md"
  put docs/README.md '# docs

- [overview](overview.md)
- [a](architecture/a.md)
- [g](guides/g.md)
- [r](reference/r.md)
- [ADR](adr/)
- [helper](../.claude/agents/helper.md)'
  put docs/overview.md '# O

<a id="map-top"></a>

## 地図

| 項目 | 親 | 置き場所 | 前提にする |
| --- | --- | --- | --- |
| repo | | `../README.md`, `../CLAUDE.md`, `./`, `../.claude/` | |
| p | | `../p/` | |'
  put docs/architecture/a.md '# A

## 構成要素

### x

- **責務**：x を持つ。'
  put docs/guides/g.md '# G

g をする手順。

1. 書く。'
  put docs/reference/r.md '# R

<a id="part"></a>

## 節

## Plain Heading'
  put docs/adr/0001-first.md '---
status: accepted
date: 2026-01-01
---

# 0001. First'
  put .claude/agents/helper.md '---
name: helper
---

補助のエージェント。'
  put p/.claude-plugin/plugin.json '{}'
  put p/docs/README.md '# p

- [overview](overview.md)
- [do](guides/do.md)
- [part](guides/part.md)
- [role](../agents/role.md)
- [rules](../rules/)'
  put p/docs/overview.md '# P'
  put p/docs/guides/do.md '# Do

目的の手順で、新しい依頼は振り分けから入る。

1. **始めに読む**：読む。
2. **作る**：作る。'
  put p/docs/guides/part.md '# Part

共通の手順で、目的の手順の段の中で使う。

1. **書く**：書く。'
  put p/agents/role.md '---
name: role
description: 試すためのロール。
---

## 受け持つ手順

書ける範囲：なし。

## 固有の振る舞い

- 答える。'
  put p/rules/rule.md '# Rule

- 守る。'
}

# A line of the given display width: wide characters, and one narrow one when the width is odd.
wide() { printf '字%.0s' $(seq $(($1 / 2))); ((($1 % 2) == 0)) || printf 'a'; }

fresh
passes "a repository that keeps every check passes"

echo "# 文書の根"
fresh
put docs/notes.txt 'x'
fails "an entry outside the layout in a docs root fails" "docs/notes.txt: not part of the docs layout"
fresh
put docs/.DS_Store 'x'
passes "a .DS_Store in a docs root passes"
fresh
put docs/reference/unlisted.md '# U'
add README.md '[u](docs/reference/unlisted.md)'
add CLAUDE.md '[u](docs/reference/unlisted.md)'
fails "a document not listed in its index fails" "docs/reference/unlisted.md: not listed in docs/README.md"
fresh
put docs/adr/0002-second.md '---
status: accepted
date: 2026-01-01
---

# 0002. Second'
passes "an ADR not listed in the index passes"

echo "# 入口からの道"
fresh
put README.md '# r

[docs](docs/README.md)'
fails "a document not reachable from README.md fails" "p/docs/README.md: not reachable by links from README.md"
fresh
put CLAUDE.md '# r

[docs](docs/README.md)'
fails "a document not reachable from CLAUDE.md fails" "p/docs/README.md: not reachable by links from CLAUDE.md"
fresh
rm "$repo/CLAUDE.md"
sed -i 's|, `../CLAUDE.md`||' "$repo/docs/overview.md"
passes "a repository without CLAUDE.md is checked from README.md alone"

echo "# 構成要素の説明"
fresh
put docs/architecture/a.md '# A

### x

- **責務**：x を持つ。'
fails "an architecture document without 構成要素 fails" "docs/architecture/a.md: no '## 構成要素' section"
fresh
put docs/architecture/a.md '# A

```
## 構成要素
```'
fails "a 構成要素 heading in fenced code does not count" "docs/architecture/a.md: no '## 構成要素' section"
fresh
add docs/architecture/a.md '
### y

- **関係**：x を前提にする。'
fails "a component without 責務 fails" "docs/architecture/a.md: component 'y' has no 責務"

echo "# 手順"
fresh
put p/docs/guides/do.md '# Do

新しい依頼は振り分けから入る。

1. **始めに読む**：読む。'
fails "a plugin's procedure that tells no layer fails" "p/docs/guides/do.md: a plugin's procedure tells its layer"
fresh
put p/docs/guides/do.md '# Do

目的の手順で、共通の手順でもある。新しい依頼は振り分けから入る。

1. **始めに読む**：読む。'
fails "a procedure of both layers fails" "p/docs/guides/do.md: a procedure is either a purpose one or a common one"
fresh
put p/docs/guides/do.md '# Do

目的の手
順で、新しい依
頼は振り分けから入る。

1. **始めに読む**：読む。'
passes "a layer and a new request told across folded lines pass"
fresh
put p/docs/guides/do.md '# Do

目的の手順で、新しい依頼は振り分けから入る。

1. **作る**：作る。'
fails "a purpose procedure that does not start by reading fails" "p/docs/guides/do.md: a purpose procedure starts with the step"
fresh
put p/docs/guides/do.md '# Do

目的の手順で、振り分けから入る。

1. **始めに読む**：読む。'
fails "a purpose procedure that tells no new request fails" "p/docs/guides/do.md: a purpose procedure tells, before its steps,"
fresh
put p/docs/guides/part.md '# Part

共通の手順で、目的の手順の段の中で使う。

1. **始めに読む**：読む。'
fails "a common procedure with a step that reads fails" "p/docs/guides/part.md: a common procedure has no step '始めに読む'"
fresh
add p/docs/guides/part.md '2. **戻る**：[do](do.md "在りか") に戻る。'
fails "a common procedure that points to another procedure fails" "p/docs/guides/part.md: a common procedure points to another procedure do.md"
fresh
add p/docs/guides/part.md '2. **引く**：[索引](../README.md) を引く。'
passes "a common procedure that points outside the procedures passes"
fresh
put p/docs/guides/sub/y.md '# Y

共通の手順で、目的の手順の段の中で使う。

1. **戻る**：[do](../do.md) に戻る。'
add p/docs/README.md '- [y](guides/sub/y.md)'
fails "a common procedure under guides/ that points to another procedure fails" "p/docs/guides/sub/y.md: a common procedure points to another procedure ../do.md"
fresh
put p/docs/guides/sub/z.md '# Z

共通の手順で、目的の手順の段の中で使う。'
add p/docs/README.md '- [z](guides/sub/z.md)'
add p/docs/guides/part.md '2. **引く**：[下の文書](sub/) を引く。'
passes "a common procedure that points to a directory under guides/ passes"
fresh
add p/docs/guides/part.md '2. **戻る**：[この手順](part.md) の 1 に戻る。'
passes "a common procedure that points into itself passes"
fresh
put p/docs/guides/sub/x.md '# X

x をする手順。'
add p/docs/README.md '- [x](guides/sub/x.md)'
fails "a plugin's procedure in a directory under guides/ tells its layer" "p/docs/guides/sub/x.md: a plugin's procedure tells its layer"
fresh
put docs/guides/g.md '# G

目的の手順で、新しい依頼は振り分けから入る。

1. **書く**：書く。'
fails "a procedure outside plugins that tells its layer keeps that layer's form" "docs/guides/g.md: a purpose procedure starts with the step"
fresh
add p/docs/guides/do.md '
```
1. x
```'
passes "a numbered line in fenced code is no step"
fresh
put p/docs/guides/do.md '# Do

目的の手順で、新しい依頼は振り分けから入る。

1. **作る**：作る。
2. **始めに読む**：読む。'
fails "a purpose procedure that reads in a later step fails" "p/docs/guides/do.md: a purpose procedure starts with the step"
fresh
put p/docs/guides/do.md '# Do

目的の手順で、振り分けから入る。

1. **始めに読む**：新しい依頼を読む。'
fails "a purpose procedure that tells a new request only in its steps fails" "p/docs/guides/do.md: a purpose procedure tells, before its steps,"
fresh
add p/docs/guides/do.md '3. 名前のない段。'
fails "a step without a bold name fails" "p/docs/guides/do.md: a step is numbered and named in bold"

echo "# 役割ごとの指示"
fresh
put p/agents/role.md '---
name: role
---

## 使う決まり

書ける範囲：なし。

## 固有の振る舞い'
fails "a role that does not open with the procedures it takes fails" "p/agents/role.md: the first section of a role's instructions is"
fresh
put p/agents/role.md '---
name: role
---

## 受け持つ手順

- 答える。

## 固有の振る舞い'
fails "a role that does not tell what it may write fails" "p/agents/role.md: '## 受け持つ手順' tells what the role may write"
fresh
add p/agents/role.md '
## 使う決まり'
fails "a role that does not close with its own ways fails" "p/agents/role.md: the last section of a role's instructions is"
fresh
put .claude/agents/helper.md '---
name: helper
---

このリポジトリの reviewer ロールとして、差分を読む。

## 読むもの'
fails "a repository's agent that says it is a role keeps a role's form" ".claude/agents/helper.md: the first section of a role's instructions is"
fresh
add .claude/agents/helper.md '
## 読むもの'
passes "a repository's helper agent is not held to a role's form"
fresh
put p/agents/role.md '---
name: role
---

## 受け持つ手順

- 答える。

## 固有の振る舞い

書ける範囲：なし。'
fails "a role that tells what it may write outside 受け持つ手順 fails" "p/agents/role.md: '## 受け持つ手順' tells what the role may write"
fresh
add .claude/agents/helper.md '
```
reviewer ロールとして
```

## 読むもの'
passes "an agent that says it is a role only in fenced code is no role"
fresh
add .claude/agents/helper.md '
## 読むもの

reviewer ロールとして読む。'
passes "an agent that says it is a role only after its first section is no role"
fresh
add p/agents/role.md '
```
## 使う決まり
```'
passes "a heading in fenced code is no section of a role"
fresh
put docs/reference/role-doc.md '# R

reviewer ロールとして読む。

## 読むもの'
add docs/README.md '- [rd](reference/role-doc.md)'
passes "a document outside agents/ that says it is a role is no role"

echo "# ADR"
adr() { put "docs/adr/$1" "$2"; }
fresh
adr first.md '# First'
fails "an ADR named without its number fails" "docs/adr/first.md: an ADR file is named NNNN-<slug>.md"
fresh
adr 0001-again.md '---
status: accepted
date: 2026-01-01
---

# 0001. Again'
fails "two ADRs of one number fail" "number 0001 is also used by"
fresh
adr 0001-first.md '# 0001. First'
fails "an ADR without frontmatter fails" "docs/adr/0001-first.md: no frontmatter"
fresh
adr 0001-first.md '---
status: accepted
date: 2026-01-01
owner: me
---

# 0001. First'
fails "an ADR with an unknown key fails" "docs/adr/0001-first.md: unknown key owner"
fresh
adr 0001-first.md '---
status: proposed
date: 2026-01-01
---

# 0001. First'
fails "an ADR of an unknown status fails" "docs/adr/0001-first.md: status must be one of"
fresh
adr 0001-first.md '---
status: superseded
date: 2026-01-01
---

# 0001. First'
fails "a superseded ADR without superseded_by fails" "docs/adr/0001-first.md: status superseded goes with superseded_by"
fresh
adr 0001-first.md '---
status: accepted
date: 2026-01-01
superseded_by: [2]
---

# 0001. First'
adr 0002-second.md '---
status: accepted
date: 2026-01-02
supersedes: [1]
---

# 0002. Second'
fails "an ADR not superseded with superseded_by fails" "docs/adr/0001-first.md: status superseded goes with superseded_by"
fresh
adr 0001-first.md '---
status: accepted
date: 2026/01/01
---

# 0001. First'
fails "an ADR dated in another form fails" "docs/adr/0001-first.md: date must be YYYY-MM-DD"
fresh
adr 0001-first.md '---
status: accepted
date: 2026-01-01
---

# First'
fails "an ADR whose title lacks its number fails" "docs/adr/0001-first.md: the title must be the first heading"
fresh
adr 0001-first.md '---
status: accepted
date: 2026-01-01
---

# 0002. First'
fails "an ADR whose title has another number fails" "docs/adr/0001-first.md: the title must be the first heading and start with '# 0001. '"
fresh
adr 0001-first.md '---
status: accepted
date: 2026-01-01
amends: 2
---

# 0001. First'
fails "a reference not written as a list fails" "docs/adr/0001-first.md: amends must be a list"
fresh
adr 0001-first.md '---
status: accepted
date: 2026-01-01
amends: [two]
---

# 0001. First'
fails "a reference neither a number nor a quoted path fails" "docs/adr/0001-first.md: amends entry two must be a number"
fresh
adr 0001-first.md '---
status: accepted
date: 2026-01-01
amends: [2]
---

# 0001. First'
fails "a reference to an ADR that does not exist fails" "docs/adr/0001-first.md: amends 2 does not exist"
fresh
adr 0002-second.md '---
status: accepted
date: 2026-01-02
supersedes: [1]
---

# 0002. Second'
fails "a reference without the one back fails" "supersedes 1, but it lacks superseded_by back to this ADR"
fresh
adr 0001-first.md '---
status: superseded
date: 2026-01-01
superseded_by: [2]
---

# 0001. First'
adr 0002-second.md '---
status: accepted
date: 2026-01-02
supersedes: [1]  # replaces the first
---

# 0002. Second'
passes "references answered in both ADRs pass"
fresh
adr 0001-first.md '---
status: accepted
date: 2026-01-01
amended_by: ["../../p/docs/adr/0001-p.md"]
---

# 0001. First'
put p/docs/adr/0001-p.md '---
status: accepted
date: 2026-01-02
amends: ["../../../docs/adr/0001-first.md"]
---

# 0001. P'
add p/docs/README.md '- [ADR](adr/)'
passes "references by path between two adr/ directories pass"

echo "# リンク"
fresh
add docs/reference/r.md '[x](/docs/README.md)'
fails "an absolute link fails" "docs/reference/r.md: absolute link /docs/README.md"
fresh
add docs/reference/r.md '[x](../README.md "索引")'
fails "a link titled other than 在りか fails" "docs/reference/r.md: link ../README.md with the title \"索引\""
fresh
add docs/reference/r.md '[x](https://example.com "例")'
passes "a link outside the repository may take any title"
fresh
add docs/reference/r.md '[x](missing.md "在りか")'
fails "a broken link fails, though it only shows where" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '[x]: missing.md'
fails "a broken link reference definition fails" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '[x](../guides/g.md#nope)'
fails "a link to an anchor that does not exist fails" "docs/reference/r.md: no anchor #nope in ../guides/g.md"
fresh
add docs/reference/r.md '[x](#節)'
fails "a link to a heading outside ASCII fails" "docs/reference/r.md: link #節 with an <a id>"
fresh
add docs/reference/r.md '
## Hello, World!

[x](#hello-world)'
passes "a link to an ASCII heading with punctuation passes"
fresh
put docs/reference/s.txt 'x'
add docs/reference/r.md '[s](s.txt#L1)'
passes "a link into a file that is not Markdown has its fragment unchecked"
fresh
add docs/reference/r.md '[x](#part) [y](#plain-heading) [z](r.md#part)'
passes "links to an <a id> and to an ASCII heading pass"
fresh
add docs/reference/r.md '`<a id="only-code"></a>`

[x](#only-code)'
fails "an <a id> in a code span is no anchor" "docs/reference/r.md: no anchor #only-code in docs/reference/r.md"
fresh
add docs/reference/r.md '
## Plain Heading

[x](#plain-heading-1)'
passes "a link to the second of two same ASCII headings passes"

echo "# コードブロックとコードスパン"
fresh
add docs/reference/r.md '`[x](missing.md)`

```
[x](missing.md)
```'
passes "links in code spans and fenced code are not checked"
fresh
add docs/reference/r.md '```
x
```

[x](missing.md)'
fails "a link after a closed fence is checked" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '```
[x](missing.md)'
passes "a fence never closed runs to the end"
fresh
add docs/reference/r.md '````
```
[x](missing.md)
````'
passes "a fence of four backticks closes only at four"
fresh
add docs/reference/r.md '````
[x](missing.md)
````'
passes "a fence of four backticks is fenced code"
fresh
add docs/reference/r.md '```
```js
[x](missing.md)
```'
passes "a line with an info string does not close a fence"
fresh
add docs/reference/r.md '~~~
```
[x](missing.md)
~~~'
passes "a fence of tildes does not close at backticks"
fresh
add docs/reference/r.md '- 項目

      ```
      [x](missing.md)
      ```'
passes "lines indented four columns past the text of a list item are indented code"
fresh
add docs/reference/r.md '  - 項目

    ```
    [x](missing.md)
    ```'
passes "a fence in a nested list item is fenced code"
fresh
add docs/reference/r.md '   ```
   [x](missing.md)
   ```'
passes "a fence indented three columns is fenced code"
fresh
add docs/reference/r.md '- 項目

  [x](missing.md)'
fails "a paragraph in a list item is no code" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md 'text

    [x](missing.md)

    [y](missing.md)'
passes "lines indented four columns after a blank line are indented code"
fresh
add docs/reference/r.md 'text
    ```
[x](missing.md)'
fails "a fence indented four columns outside a list opens nothing" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md 'text
    [x](missing.md)'
fails "a line indented four columns right after a paragraph is no code" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '  - 項目

    [x](missing.md)'
fails "a paragraph in a nested list item is no code" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '- 項目
  - 中の項目

  続き

      [x](missing.md)'
passes "a less indented line goes back to the list item it continues"
fresh
add docs/reference/r.md '- 項目
  - 中の項目

  続き

    [x](missing.md)'
fails "a line indented less than four past the item it goes back to is no code" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '- 項目

text
    ```
[x](missing.md)'
fails "a paragraph after a list goes back to no indentation" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '1. 項目

    [x](missing.md)'
fails "a paragraph in a numbered list item is no code" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '```
x
```
    [x](missing.md)'
passes "a line indented four columns right after a fence is indented code"
fresh
add docs/reference/r.md '```
    ```
[x](missing.md)
```'
passes "a fence line indented four columns does not close a fence"
fresh
add docs/reference/r.md '```
    ```
```

[x](missing.md)'
fails "a link after a fence with a deeply indented fence line in it is checked" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '- 項目

  ```
      ```
  [x](missing.md)
  ```'
passes "a fence line indented four columns past a list item does not close a fence"
fresh
add docs/reference/r.md '- ```md
  [x](missing.md)
  ```'
passes "a fence right after a list marker is fenced code"
fresh
add docs/reference/r.md '- ```md
  x
  ```

[x](missing.md)'
fails "a link after a fence that opens after a list marker is checked" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '`` `[x](missing.md)` ``'
passes "a code span of two backticks hides its link"
fresh
add docs/reference/r.md '`` [x](missing.md) ` ``'
passes "a code span of two backticks holds a single backtick"
fresh
add docs/reference/r.md 'a ``` [x](missing.md) `` b'
fails "runs of backticks of other lengths make no code span" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '  - 項目

    ```
    x
    ```

    [x](missing.md)'
fails "a link after a fence closed in a nested list item is checked" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '- ```
[x](missing.md)'
fails "a fence in a list item that nothing closes ends where the item ends" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '- ```
x
  [x](missing.md)'
fails "a fence that its list item ended does not take later lines back" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '- <!--
x
  [x](missing.md)'
fails "a comment that its list item ended does not take later lines back" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '  - item
    <!--
    [x](missing.md)
    -->'
passes "an HTML comment in a nested list item is code"
fresh
add docs/reference/r.md '10. 項目

     [x](missing.md)'
fails "a paragraph in an item numbered with two digits is no code" "docs/reference/r.md: broken link missing.md"
fresh
for _ in $(seq 12000); do printf -- '- ```x\n'; done >>"$repo/docs/reference/r.md"
status=0
(cd "$repo" && timeout 10 "$bin" >/dev/null 2>&1) || status=$?
if [[ $status != 124 ]]; then
  echo "ok   fences that are never closed are read in time"
else
  echo "FAIL fences that are never closed are read in time: it took more than 10 seconds"
  failed=1
fi
fresh
add docs/reference/r.md '- 例：

  ```sh
  cat <<X
[x](missing.md)
X
  ```'
passes "a fence closed later keeps less indented lines as code"
fresh
add docs/reference/r.md '- 例：

  ```sh
  cat <<X
X
  ```

[x](missing.md)'
fails "a link after a fence with less indented lines is checked" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '```
x
````

[x](missing.md)'
fails "a longer line closes a fence" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '   ```
   x
   ```

[x](missing.md)'
fails "a link after a fence closed three columns deep is checked" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '- 項目

  ```
  a

  [x](missing.md)
  ```'
passes "a blank line in a fence in a list item keeps the fence"
fresh
add docs/reference/r.md '- 項目

  ```
  a

  [x](missing.md)'
passes "a blank line in a fence in a list item that nothing closes keeps the fence"
fresh
add docs/reference/r.md '~~~ a`b
[x](missing.md)
~~~'
passes "a tilde fence may hold a backtick in its info string"
fresh
add docs/reference/r.md '<!--
```
-->

[x](missing.md)'
fails "a fence line in an HTML comment opens nothing" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '<!--
[x](missing.md)
<a id="only"></a>
-->

[y](#only)'
fails "an HTML comment is read as code" "docs/reference/r.md: no anchor #only"
fresh
add docs/reference/r.md '<!-- note -->

[x](missing.md)'
fails "a link after a one-line HTML comment is checked" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md 'text

    <!--
[x](missing.md)'
fails "an HTML comment indented four columns opens no comment" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '-   項目

      [x](missing.md)'
fails "the text of a list item begins after all the spaces after its marker" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '-      a

     [x](missing.md)'
fails "after five spaces after a marker, the text of the item begins one space after it" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '- step

  ```sh
  cat > list.yaml <<X
- name: [x](missing.md)
X
  ```'
passes "a fence closed later keeps a less indented list marker line as code"
fresh
add docs/reference/r.md '- step

  ```sh
  cat > list.yaml <<X
- name: a
X
  ```

[x](missing.md)'
fails "a link after a fence with a less indented list marker line in it is checked" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '-    a

    [x](missing.md)'
passes "a line less indented than the text of its item and four columns deep after a blank line is indented code"
fresh
add docs/reference/r.md '- a
  -    b

      [x](missing.md)'
passes "a line that leaves a nested item is judged against the item it stays in"
fresh
add docs/reference/r.md 'a `x
[x](missing.md)
b` c'
fails "a code span does not run across lines" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '- a
  ```
<!--
[x](missing.md)
-->'
passes "the line that ends a fence nothing closes is read again"
fresh
add docs/reference/r.md '```
x
```  

[x](missing.md)'
fails "a closing line may end with blanks" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '- item
  <!-- open
  [p](missing.md)
- next'
passes "an HTML comment that nothing closes is code up to the end of its list item"
fresh
add docs/reference/r.md '- item
  <!-- open
  [p](p.md)
- next [x](missing.md)'
fails "an HTML comment in a list item that nothing closes ends with the item" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '- a
  - b
  <!-- x

  [l](missing.md)'
passes "an HTML comment that nothing closes ends with the item it is in, not with an item that has ended"
fresh
add docs/reference/r.md '1. step

   text
<!-- note -->
    [x](missing.md)'
passes "a line indented four columns right after an HTML comment that ends a list is indented code"
fresh
add docs/reference/r.md '<!--
x
-->
    [x](missing.md)'
passes "a line indented four columns right after a closed HTML comment is indented code"
fresh
add docs/reference/r.md '<!-->

[x](missing.md)'
fails "<!--> is a whole HTML comment" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '<!--
x
--> tail

[x](missing.md)'
fails "a link after a comment closed before the end of a line is checked" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '## Heading
    [x](missing.md)'
passes "a line indented four columns right after a heading is indented code"
fresh
add docs/reference/r.md 'Title
=====
    [x](missing.md)'
passes "a line indented four columns right after a setext heading is indented code"
fresh
add docs/reference/r.md '* * *

    [x](missing.md)'
passes "a thematic break is no list item"
fresh
add docs/reference/r.md '-     [x](missing.md)'
passes "five spaces after a marker start indented code on the marker line"
fresh
add docs/reference/r.md 'text
    <!--
[x](missing.md)'
fails "a line indented four columns in a paragraph opens no HTML comment" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '- item
  <!--
[x](missing.md)
  -->'
passes "an HTML comment closed later keeps less indented lines as code"
fresh
add docs/reference/r.md '- item
  <!--
x
  -->

[x](missing.md)'
fails "a link after an HTML comment with less indented lines is checked" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '1) 項目

    [x](missing.md)'
fails "a paragraph in an item numbered with a parenthesis is no code" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '   [x]: missing.md'
fails "a link reference definition indented three columns is checked" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '```
<a id="only"></a>
```

[x](#only)'
fails "an <a id> in fenced code is no anchor" "docs/reference/r.md: no anchor #only"
fresh
add docs/reference/r.md '```
<a id="map-x"></a>

| a | b |
```'
passes "a map anchor in fenced code starts no map"
fresh
add docs/reference/r.md '- a
  - b

   c

      ```

  [x](missing.md)'
fails "a less indented line goes back to the text of the item it is in" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '`[x](missing.md)`` b'
fails "a longer run of backticks does not close a code span" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '`` [x](missing.md) ``` b'
fails "a run of backticks after a backtick does not close a code span" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '- a
  - b
    - c

  d

      [x](missing.md)'
passes "a line that leaves two nested items at once is judged against the item it stays in"
fresh
add docs/reference/r.md 'text
#tag
    [x](missing.md)'
fails "a hash without a space after it is no heading" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md 'text
    # h
    [x](missing.md)'
fails "a hash indented four columns in a paragraph is no heading" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '+ 項目

    [x](missing.md)'
fails "a plus sign marks a list item" "docs/reference/r.md: broken link missing.md"
fresh
put docs/reference/r.md '    [x](missing.md)'
passes "a line indented four columns at the start of a document is indented code"
fresh
add docs/reference/r.md 'Title
--
    [x](missing.md)'
passes "a line indented four columns right after a setext heading of dashes is indented code"
fresh
add docs/reference/r.md '-     [x](missing.md)
      [y](missing.md)'
passes "indented code that begins on a marker line goes on in the next line"
fresh
add docs/reference/r.md '`` [x](missing.md)'
fails "two backticks never closed make no code span" "docs/reference/r.md: broken link missing.md"
fresh
add docs/reference/r.md '```x`y
[x](missing.md)'
fails "a backtick line whose info string holds a backtick opens no fence" "docs/reference/r.md: broken link missing.md"

echo "# 量の上限"
fresh
for i in $(seq 57); do add CLAUDE.md "- $i" && add CLAUDE.md ''; done
passes "CLAUDE.md of 60 lines, blank lines not counted, passes"
add CLAUDE.md '- 58'
fails "CLAUDE.md of 61 lines fails" "CLAUDE.md: 61 lines, more than the 60 lines"
fresh
for i in $(seq 38); do add p/rules/more.md "- $i"; done
passes "rules of 40 lines in all pass"
add p/rules/more.md '- 39'
fails "rules of 41 lines in all fail" "p/rules: 41 lines, more than the 40 lines"
fresh
for i in $(seq 46); do add p/agents/role.md "- $i"; done
passes "a role of 50 lines, frontmatter not counted, passes"
add p/agents/role.md '- 47'
fails "a role of 51 lines fails" "p/agents/role.md: 51 lines, more than the 50 lines"
fresh
add CLAUDE.md "$(wide 130)"
passes "a line 130 columns wide passes"
fresh
add CLAUDE.md "$(wide 131)"
fails "a line 131 columns wide fails" "CLAUDE.md:5: 131 columns wide, more than the 130"
fresh
add CLAUDE.md "[$(wide 126)](docs/README.md \"在りか\")"
passes "a line long only by the parentheses of a link passes"
fresh
add CLAUDE.md "[$(wide 120)](docs/README.md \"在りか\") \`x\`"
passes "the parentheses of a link before a code span are not counted"
fresh
add CLAUDE.md "\`[$(wide 126)](docs/README.md)\`"
fails "the parentheses of a link in a code span are counted" "CLAUDE.md:5: 146 columns wide"
fresh
add CLAUDE.md "\`\`\`
[$(wide 126)](docs/README.md)
\`\`\`"
fails "the parentheses of a link in fenced code are counted" "CLAUDE.md:6: 144 columns wide"
fresh
add CLAUDE.md "\`\`\`[$(wide 126)](docs/README.md)"
fails "the line that opens a fence is counted as code" "CLAUDE.md:5: 147 columns wide"
fresh
add CLAUDE.md "$(printf 'Ａ%.0s' $(seq 65))a"
fails "a fullwidth character counts two columns" "CLAUDE.md:5: 131 columns wide"
fresh
put p/agents/role.md "---
name: role
description: $(wide 200)
---

## 受け持つ手順

書ける範囲：なし。

## 固有の振る舞い"
passes "a long line in the frontmatter passes"
fresh
put p/agents/role.md "---
name: role
description: 試すためのロール。
---

## 受け持つ手順

書ける範囲：なし。

## 固有の振る舞い

$(wide 131)"
fails "a line after the frontmatter is told by its number in the file" \
  "p/agents/role.md:12: 131 columns wide"
fresh
add .claude/agents/helper.md "$(wide 131)"
fails "a repository's agent is held to the width" ".claude/agents/helper.md:6: 131 columns wide"
fresh
add p/rules/rule.md "$(wide 131)"
fails "rules are held to the width" "p/rules/rule.md:4: 131 columns wide"
fresh
add docs/reference/r.md "$(wide 200)"
passes "a document not read at every start is not held to the width"

echo "# 地図"
top='<a id="map-top"></a>

| 項目 | 親 | 置き場所 | 前提にする |
| --- | --- | --- | --- |'
fresh
add docs/overview.md '| p | | `../p/docs/` | |'
fails "an item declared twice fails" "docs/overview.md: item p is already declared"
fresh
add docs/overview.md '| q | | | nope |'
fails "a prerequisite that is no item fails" "docs/overview.md: item q may only rely on its siblings, not nope"
fresh
add docs/overview.md '| q | x | | |'
fails "an item of an unknown parent fails" "docs/overview.md: item q has unknown parent x"
fresh
add docs/overview.md '| q | | `../q.md` | |'
fails "a place that does not exist fails" "docs/overview.md: place ../q.md of item q does not exist"
fresh
add docs/overview.md '| q | | | p |
| r | q | | p |'
fails "a prerequisite that is no sibling fails" "docs/overview.md: item r may only rely on its siblings, not p"
fresh
add docs/overview.md '| q | | | r |
| r | | | q |'
fails "prerequisites in a cycle fail" "docs/overview.md: item q relies on itself through a cycle"
fresh
add docs/overview.md '| q | r | | |
| r | q | | |'
fails "parents in a cycle fail" "docs/overview.md: item q is its own ancestor"
fresh
add docs/overview.md '| c | | | |
| c1 | c | | |'
passes "an item inside an item without a place is mapped where that item is declared"
fresh
add docs/overview.md '| c | | | |'
put p/docs/overview.md "# P

${top/top/p}
| c1 | c | | |"
fails "an item inside an item without a place mapped elsewhere fails" "p/docs/overview.md: item c1 belongs in the map that declares c"
fresh
put p/docs/overview.md "# P

${top/top/p}
| p-guides | p | \`guides/\` | |"
passes "an item inside an item is mapped in that item's overview"
fresh
put p/docs/overview.md "# P

${top/top/p}
| p-guides | p | \`guides/\` | |"
add p/docs/guides/do.md '3. **引く**：[全体](../overview.md) を引く。'
passes "an item inside an item may rely on the whole of its parent"
fresh
put p/docs/overview.md "# P

[do](guides/do.md)

${top/top/p}
| p-guides | p | \`guides/\` | |"
fails "the whole of an item may not rely on an item inside it" "p/docs/overview.md: relies on guides/do.md, but p may not rely on p-guides"
fresh
add docs/overview.md '| p-guides | p | `../p/docs/guides/` | |'
fails "an item inside an item mapped above it fails" "docs/overview.md: item p-guides belongs in a map in p's own documents"
fresh
put p/docs/overview.md "# P

${top/top/p}
| q | | | |"
fails "top-level items in two maps fail" "top-level items must be declared in exactly one table"
fresh
mv "$repo/docs/overview.md" "$repo/p/docs/overview.md"
sed -i -e 's#`\.\./README\.md`, `\.\./CLAUDE\.md`, `\./`, `\.\./\.claude/`#`../../README.md`, `../../CLAUDE.md`, `../../docs/`, `../../.claude/`#' \
  -e 's#`\.\./p/`#`../`#' "$repo/p/docs/overview.md"
add p/docs/README.md '- [top](../../docs/README.md)'
put docs/README.md '# docs

- [a](architecture/a.md)
- [g](guides/g.md)
- [r](reference/r.md)
- [ADR](adr/)
- [helper](../.claude/agents/helper.md)'
fails "top-level items mapped outside docs/overview.md fail" "p/docs/overview.md: top-level items are declared in docs/overview.md"
fresh
add docs/reference/r.md "
${top/top/r}
| q | | | |"
fails "a map outside an overview fails" "docs/reference/r.md: a map sits in the docs/overview.md of the item it maps"
fresh
put docs/architecture/overview.md "# AO

## 構成要素

### y

- **責務**：y を持つ。

${top/top/ao}
| q | | | |"
add docs/README.md '- [ao](architecture/overview.md)'
fails "a map in an overview outside a docs root fails" "docs/architecture/overview.md: a map sits in the docs/overview.md"
fresh
put q/docs/overview.md "# Q

${top/top/q}
| q-in | p | | |"
add p/docs/README.md '- [q](../../q/docs/overview.md)'
fails "a map in a docs/ without an index fails" "q/docs/overview.md: a map sits in the docs/overview.md"
fresh
add docs/README.md "
${top/top/idx}
| q | | | |"
fails "a map in an index fails" "docs/README.md: a map sits in the docs/overview.md"
fresh
add docs/reference/r.md 'The form `<a id="map-x"></a>` starts a map.'
passes "a map anchor in a code span starts no map"
fresh
add docs/overview.md '```
x
```
| q | | `../missing.md` | |'
passes "a code block ends a map's table"
fresh
add docs/reference/r.md '
<a id="map-r"></a>

| 項目 | 親 | 置き場所 |
| --- | --- | --- |'
fails "a map without the columns fails" "docs/reference/r.md: the map after <a id=\"map-r\"></a> needs a table with the columns"
fresh
put other.md '# Other'
add README.md '[o](other.md)'
add CLAUDE.md '[o](other.md)'
fails "a document in no item fails" "other.md: not in any item of the maps"
fresh
add p/docs/guides/do.md '3. **引く**：[参照](../../../docs/reference/r.md) を引く。'
fails "a link against the prerequisites fails" "p/docs/guides/do.md: relies on ../../../docs/reference/r.md, but p may not rely on repo"
fresh
add p/docs/guides/do.md '3. **引く**：[参照](../../../docs/reference/r.md "在りか") を引く。'
passes "a link that only shows where is not held to the prerequisites"
fresh
sed -i 's/^| p | | `..\/p\/` | |$/| p | | `..\/p\/` | repo |/' "$repo/docs/overview.md"
add p/docs/guides/do.md '3. **引く**：[参照](../../../docs/reference/r.md) を引く。'
passes "a link along the prerequisites passes"
fresh
add docs/reference/r.md '[do](../../p/docs/guides/do.md) [adr](../adr/0001-first.md)'
add docs/adr/0001-first.md '[do](../../p/docs/guides/do.md)'
fails "a link to a sibling not relied on fails" "docs/reference/r.md: relies on ../../p/docs/guides/do.md, but repo may not rely on p"
fresh
add docs/adr/0001-first.md '[do](../../p/docs/guides/do.md)'
passes "links from an ADR and from an index are not held to the prerequisites"
fresh
add p/docs/guides/do.md '3. **引く**：[判断](../../../docs/adr/0001-first.md) を引く。'
passes "a link to an ADR is not held to the prerequisites"
fresh
put docs/overview.md '# O'
put other.md '# Other'
add README.md '[o](other.md)'
add CLAUDE.md '[o](other.md)'
passes "a repository without maps has no map checks"

echo "# 検査する文書"
# Without the signing and hooks of the git configuration of whoever runs the test.
commit() {
  git -C "$repo" add -A
  git -C "$repo" -c user.name=t -c user.email=t@example.com -c commit.gpgsign=false -c core.hooksPath=/dev/null \
    commit --quiet -m c
}
fresh
add docs/reference/r.md '[x](missing.md)'
commit
fails "a committed document is checked" "docs/reference/r.md: broken link missing.md"
fresh
put docs/adr/0002-gone.md '[x](missing.md)'
commit
rm "$repo/docs/adr/0002-gone.md"
passes "a committed document removed from the work tree is not checked"
fresh
printf 'ignored/\n' >"$repo/.gitignore"
put ignored/x.md '[x](missing.md)'
passes "a Markdown file git ignores is not checked"
fresh
rm -rf "$repo/.git"
passes "outside a git work tree, the documents under the working directory pass"
put node_modules/x/README.md '[x](missing.md)'
put .git/x.md '[x](missing.md)'
passes "outside a git work tree, node_modules and .git are not checked"
put docs/reference/x.md '[x](missing.md)'
fails "outside a git work tree, every other Markdown file is checked" "docs/reference/x.md: broken link missing.md"
fresh
add docs/reference/r.md '[x](missing.md)'
fails "from a directory inside the repository, the whole repository is checked" "docs/reference/r.md: broken link missing.md" \
  docs/guides

exit "$failed"
