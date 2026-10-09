# 0009. コンテナは root を使わず、ビルドのときに決めた数値の UID で動かす

- Status: Accepted
- Date: 2026-10-08

## Context

devcontainer とエージェントのコンテナは root で起動し、entrypoint が `node` の UID と GID をリポジトリの所有者に
合わせ直し、ボリュームの所有者を直してから、コマンドを実行していた（ADR 0001）。devcontainer では `sleep` が、
エージェントでは `runuser` が root のまま PID 1 として残っていた。UID と GID はビルドのときにホストの値に合わせる
ようになり、起動時の調整は使われなくなっていた。Kubernetes などでは、UID と GID は Pod の設定で決め、ボリュームの
所有者は `fsGroup` で合わせる。`runAsNonRoot` で root でないことを確かめるには、イメージのユーザーが数値である
必要がある。

## Decision

- イメージのユーザーは、ビルドのときに `HOST_UID` / `HOST_GID` で決めた数値の UID と GID の `node` にする。
  コンテナの中で root は使わない。
- 起動時の UID の合わせ直しとボリュームの所有者の直しをなくす。ボリュームのマウント先はイメージの中に `node` の
  持ち物として作っておく。
- entrypoint は、作業ディレクトリに書き込めないときに原因と直し方を知らせるだけにする。
- control-plane は `claude ...` をそのままコマンドとして渡す。
- 人が root で作業したいときは `docker exec -u root` で入る。

## Alternatives

- **root で起動し、entrypoint で調整してから `node` に権限を落とす**：調整はビルドのときに済んでいて、
  起動のたびに root を使う理由がない。Kubernetes では root での調整に頼れない。
- **ユーザーを名前（`node`）で指定する**：プラットフォームが root でないことを確かめられない。

## Consequences

- イメージは、リポジトリの所有者の UID と GID でビルドする必要がある。違っていると、作業ディレクトリやボリュームに
  書き込めない（entrypoint が知らせる）。
- 以前に root の所有で作られたボリュームは、一度だけ `docker exec -u root` で所有者を直すか、作り直す。
