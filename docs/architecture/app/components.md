# control-plane の構成

control-plane のプロセス `acp serve` の中にある構成要素と、依頼がそれらをどう流れるか。コードの層と依存の向きは
[コードの構成](../codebase/layers.md)に、依頼の入口と知らせの中身は[依頼と知らせ](requests.md)にある。

## 構成

`acp serve` は常駐するサーバーで、`compose.yaml` で Docker のプロキシと並べて動かす
（[ADR 0014](../../adr/app/0014-control-plane-as-a-server.md)）。依頼は Slack の Bot か HTTP API から入り、
ユースケースを通ってセッションのワーカーに届き、ワーカーがエージェントのコンテナを動かす。

```
Slack ──Socket Mode──▶ slackbot.Bot ──▶ usecase.HandleMessage ──┐
acp run / stop ──HTTP──▶ httpapi ───────────────────────────────┤
                                                                ▼
                                                      usecase.Dispatcher
                                                        └ worker（セッションごと、受信箱）
                                                              │ Runner
                                                              ▼
                                       docker.Runner ──▶ docker-proxy ──▶ エージェントのコンテナ

usecase.SessionCleaner（起動時と定期的に）

知らせ：worker ──SessionObserver──▶ sessionObserver（serve.go）──▶ slackbot.Bot
       SessionCleaner ──SessionNotifier──▶ sessionObserver ──▶ slackbot.Bot
```

- **入口**は依頼を受け取って、ユースケースの入力に直すだけ。チャットからの依頼は受信方式に依存しない
  `HandleMessage` が受け、HTTP API は `Dispatcher` を直接呼ぶ。
- **ユースケース**は判定と手順を持ち、外部とはポート（インターフェース）越しにだけ話す。`Dispatcher` は
  セッションごとにワーカーを一つ作り、ワーカーが Claude Code のプロセスと依頼の順番を持つ
  （[ADR 0017](../../adr/app/0017-long-lived-session-process.md)）。
- **アダプター**はポートを一つの技術で実装する。どのポートをどれで満たすかは `cmd/acp/serve.go` が決める。
- **知らせ**は、依頼への返事なら入口が返し、依頼のないところで起きたことは `sessionObserver` が Bot に渡す
  （[依頼と知らせ](requests.md)）。

ポートと、それを満たすもの：

| ポート | 定義 | 満たすもの |
| --- | --- | --- |
| `AgentRegistry`、`People` | `ports.go` | `config.Registry`、`config.People` |
| `AccessPolicy` | `handle_message.go` | `config.AccessList` |
| `SessionRepository` | `ports.go` | `filesession.Store` |
| `WorkspaceProvider`、`SessionWorkspace` | `ports.go` | `gitworkspace.Provider` |
| `CredentialIssuer` | `ports.go` | `serve.go` の `credentialIssuers`（`claudeauth.FileIssuer`、`githubapp.Issuer`、`slackauth.Issuer` をまとめる） |
| `Runner`、`Process` | `ports.go` | `docker.Runner` |
| `SessionFiles` | `ports.go` | `docker.SessionFiles` |
| `SessionBranches` | `ports.go` | `githubapp.Branches` |
| `SessionObserver`、`SessionNotifier` | `ports.go` | `serve.go` の `sessionObserver` |
| `ChatResponder` | `handle_message.go` | `slackbot.Bot` |
| `TurnExecutor` | `handle_message.go` | `usecase.Dispatcher` |

セッションの段階と置き場所は[セッション](sessions.md)、認証情報は[認証情報](credentials.md)、エージェントの
コンテナは[コンテナ](containers.md)、作業領域は[作業領域](workspace.md)にある。

## 構成要素

各構成要素の中身は、リンク先の文書にある。

### `cmd/acp/serve.go`

- **責務**：`acp serve` として、設定を読み、各部品を組み立て、起動と停止の順番を決める。
- **置くもの / 置かないもの**：ポートとアダプターのつなぎ方、間隔や時間の値（値はそれを使う部品の文書にある）、
  複数のアダプターを一つのポートにまとめる小さな型（`credentialIssuers`、`sessionObserver`）。`Dispatcher` と Bot が
  互いを要る循環は、`sessionObserver` に Bot を後から渡すことでここだけで解く。判定は置かない。
- **関係**：すべてのパッケージに依存してよい。起動は、`Dispatcher` を作り、Bot を作って `sessionObserver` に渡し、
  前の実行が残したコンテナを `Dispatcher.Recover` で片付けてから、HTTP サーバー、Bot、片付けを動かす。停止は、
  Bot が受付をやめたあと `Dispatcher.Close` で待っている依頼を終わらせ、HTTP サーバーと Bot が返事を書き終える
  のを待つ（[依頼と知らせ](requests.md#shutdown)）。
- **当てはめた原則・パターン**：[依存の向き](../../../platform/docs/reference/principles.md#dependency-rule)：組み立てを外側の
  一か所に集め、ユースケースはアダプターを知らない。

### `cmd/acp`（`main.go`、`run.go`、`slackauth.go`）

- **責務**：サブコマンドの振り分けと、HTTP API のクライアントとしての `acp run` と `acp stop`、Slack のトークンを
  取り直す `acp slack-auth`。
- **関係**：`run` と `stop` は `httpapi.Client` を使う。使い方は[依頼の仕方](../../reference/usage.md)、
  `slack-auth` は[認証情報](credentials.md)と[運用](../../guides/operations.md)にある。

### `internal/usecase/ports.go`

- **責務**：ユースケースが外部に求めるインターフェースと、それに渡す値の型。
- **置くもの / 置かないもの**：チャットの入口に関わるポート（`AccessPolicy`、`ChatResponder`、`TurnExecutor`）は
  ここではなく `handle_message.go` に置く。
- **当てはめた原則・パターン**：[依存の向き](../../../platform/docs/reference/principles.md#dependency-rule)：アダプターは
  ここに合わせて作る。

### `internal/adapter/config`

- **責務**：設定ファイル（`acp.yaml`）を読み、検証して、エージェントの一覧、依頼者の許可、依頼者とコミットの
  作者の対応を返す。
- **置くもの / 置かないもの**：ファイルの形、既定値、読むときの検証。設定の項目の説明は
  [acp.example.yaml](../../../acp.example.yaml) に置く。
- **関係**：`AgentRegistry`、`AccessPolicy`、`People` を満たす。
- **当てはめた原則・パターン**：[仕組みで守る](../../../platform/docs/reference/principles.md#mechanisms)：知らない項目や
  相対パスは、起動の前に読むところで弾く。

### `internal/adapter/slackbot`

- **責務**：エージェント一つの Slack アプリとして依頼を受け、スレッドに返事と状態メッセージを書く
  （[依頼と知らせ](requests.md)）。

### `internal/adapter/httpapi`

- **責務**：HTTP API のサーバーとクライアント（[依頼と知らせ](requests.md#http-api)）。

### `internal/usecase/handle_message.go`

- **責務**：チャットから届いた依頼を、受信方式に関係なく扱う（[依頼と知らせ](requests.md)）。

### `internal/usecase/dispatcher.go`

- **責務**：アクティブなセッションごとにワーカーを動かし、依頼の順番を決めてターンの結果を返す。依頼の順番は
  [依頼と知らせ](requests.md)、ワーカーの段階は[セッション](sessions.md#worker)、認証情報の更新は
  [認証情報](credentials.md)にある。

### `internal/usecase/clean_sessions.go`

- **責務**：使われていないセッションを片付ける（[セッション](sessions.md#cleaner)）。

### `internal/adapter/filesession`

- **責務**：セッションの記録とロック（[セッション](sessions.md#record)）。

### `internal/adapter/gitworkspace`

- **責務**：セッションの作業領域を作り、その状態を返す（[作業領域とブランチ](workspace.md)）。

### `internal/adapter/docker`

- **責務**：エージェントのコンテナを動かし、Claude Code とやり取りする（[コンテナ](containers.md)）。

### `internal/adapter/githubapp`、`internal/adapter/slackauth`、`internal/adapter/claudeauth`

- **責務**：エージェントの認証情報を発行する（[認証情報](credentials.md)）。`githubapp` はリモートのブランチも
  調べて消す（[作業領域とブランチ](workspace.md#github-branches)）。

### `compose.yaml`

- **責務**：`control-plane` と `docker-proxy` の二つのサービスと、ボリューム、ネットワークを宣言する
  （[コンテナ](containers.md)）。準備と起動の手順は[運用](../../guides/operations.md)にある。
