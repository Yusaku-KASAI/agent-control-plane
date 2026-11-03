# ターゲットをひな形の新しい版に合わせる

写して作ったターゲットを、target-template（写す元の `target/`）の新しい版に合わせる手順。何をどう合わせるかの判断は
[ADR 0021](../adr/0021-update-targets-by-three-way-merge.md) に、ファイルごとの共通と埋める部分は
[ファイルの分類](../overview.md#files)にある。

## 1. 合わせる前の版を確かめる

ターゲットの `template/base` に、写したか前に合わせたときの agent-control-plane のコミットがある。ないターゲット
（`template/base` を書く前に写したもの）は、写したときのコミットを探して 2 段で `--base` に渡す。写す元が `target/` の
形になる前のコミットは渡せない。そのターゲットは、一度 `target/` と比べて手で合わせ、合わせた版を `template/base` に
書いてから使う。

## 2. 合わせる

ターゲットの変更をコミットしてから、agent-control-plane の合わせたい版（ふつうは `main`、変更のない作業ツリー）で
回す。`template/base` より古い版からは回せない。

```bash
<agent-control-plane>/target/template/bin/update-target [--base <コミット>] <ターゲット>
```

写す元のファイル（`template/` の外。`.tmpl` は外した名前）ごとに、ターゲットのファイル、前の版、今の版を三方向で
マージする。ターゲットが消したファイルは足さず、ターゲット自身の文書（`README.md` と `docs/` のひな形）はマージしない
（扱いは ADR 0021）。`template/` の文書は今の版に差し替え（外へのリンクの書き換えは
[新しいターゲットを作る](new-target.md)と同じ）、`template/base` を今の版にする。

## 3. ぶつかった所と消えたファイルを直す

`update-target` が挙げたファイルの `<<<<<<< target` から `>>>>>>> template` までを、ターゲットの部分（埋めた所）と
ひな形の変更の両方が残るように直す。ターゲットで消したがひな形で変わったと挙げたファイルは、ひな形の変更
（`git -C <agent-control-plane> diff <前の版> -- target/<ファイル>`）を見て、足し直すか消したままにするかを決める。
写す元から消えたと挙げたファイルは、ひな形から来たものなら消す。

## 4. 確かめてコミットする

`git status` と `git diff` で変更（足したファイルも `git diff` に出る）を読み、`make check` を通してから、PR にする。
PR の本文には、ひな形のどの版からどの版に合わせたか（`template/base` の差分）を書く。
