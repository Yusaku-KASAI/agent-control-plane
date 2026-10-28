# control-plane の構成

control-plane のプロセス `acp serve` の中にある構成要素と、依頼がそれらをどう流れるか。コードの層と依存の向きは
[コードの構成](../codebase/layers.md "在りか")に、依頼の入口と知らせの中身は[依頼と知らせ](requests.md "在りか")にある。

## 構成

`acp serve` は常駐するサーバーで、`compose.yaml` で Docker のプロキシと並べて動かす
（[ADR 0014](../../adr/app/0014-control-plane-as-a-server.md)）。依頼は Slack の Bot か HTTP API から入り、
ユースケースを通ってセッションのワーカーに届き、ワーカーがエージェントのコンテナを動かす。

```
Slack ──Socket Mode──▶ slackbot.Bot ──▶ usecase.HandleMessage ──┐
acp run / stop ──HTTP──▶ httpapi ───────────────────────────────┤
                                                                ▼
                                                      usecase.Dispatcher
                                                        └ worker（セッションごと、受信箱と待ち行列）
                                                              │ Runner
                                                              ▼
                                       docker.Runner ──▶ docker-proxy ──▶ エージェントのコンテナ

usecase.SessionCleaner（起動時と定期的に。今すぐの片付けは usecase.HandleMessage ──SessionCleanup──▶ SessionCleaner）
SessionCleaner ──SessionHolder──▶ Dispatcher（休んでいるワーカーを手放させる）

知らせ：worker ──SessionObserver──▶ sessionObserver（serve.go）──▶ slackbot.Bot
       SessionCleaner ──SessionNotifier──▶ sessionObserver ──▶ slackbot.Bot
```

- **入口**（`slackbot`、`httpapi`。コードの層ではアダプター）は依頼を受け取って、ユースケースの入力に直すだけ。チャットからの依頼は受信方式に依存しない
  `HandleMessage` が受け、HTTP API は `Dispatcher` を直接呼ぶ。
- **ユースケース**は判定と手順を持ち、外部とはポート（インターフェース）越しにだけ話す。`Dispatcher` は
  セッションごとにワーカーを一つ作り、ワーカーが Claude Code のプロセスと依頼の順番を持つ
  （[ADR 0017](../../adr/app/0017-long-lived-session-process.md)、[セッションのワーカー](sessions.md#worker "在りか")）。
  片付けは[片付け](cleanup.md "在りか")にある。
- **ほかのアダプター**はポートを一つの技術で実装する。どのポートをどれで満たすかは `cmd/acp/serve.go` が決める。
- **知らせ**は、依頼への返事なら入口が返し、依頼のないところで起きたことは `sessionObserver` が Bot に渡す
  （[依頼と知らせ](requests.md "在りか")）。

ポートと、それを満たすもの：

| ポート | 定義 | 満たすもの |
| --- | --- | --- |
| `AgentRegistry`、`People` | `ports.go` | `config.Registry`、`config.People` |
| `AccessPolicy` | `handle_message.go` | `config.AccessList` |
| `SessionRepository` | `ports.go` | `filesession.Store` |
| `WorkspaceProvider` | `ports.go` | `gitworkspace.Provider` |
| `SessionWorkspace` | `ports.go` | `docker.WorkspaceReader`（コンテナの中で `gitworkspace.Provider` の `State` を回す） |
| `CredentialIssuer` | `ports.go` | `serve.go` の `credentialIssuers`（`claudeauth.FileIssuer`、`githubapp.Issuer`、`slackauth.Issuer` をまとめる） |
| `Runner`、`Process` | `ports.go` | `docker.Runner` |
| `SessionFiles` | `ports.go` | `docker.SessionFiles` |
| `SessionBranches` | `ports.go` | `githubapp.Branches` |
| `SessionObserver`、`SessionNotifier` | `ports.go` | `serve.go` の `sessionObserver` |
| `UsageLog` | `ports.go` | `usagelog.Log` |
| `ChatResponder` | `handle_message.go` | `slackbot.Bot` |
| `TurnExecutor` | `handle_message.go` | `usecase.Dispatcher` |
| `SessionCleanup` | `handle_message.go` | `usecase.SessionCleaner` |
| `SessionHolder` | `ports.go` | `usecase.Dispatcher` |

ほかの部品は、それを扱う文書に構成要素として書いてある（文書の一覧は[docs/README.md](../../README.md "在りか")）。

## 構成要素

<a id="serve"></a>

### `cmd/acp/serve.go`

- **責務**：`acp serve` として、設定を読み、各部品を組み立て、起動と停止の順番を決める。
- **置くもの / 置かないもの**：ポートとアダプターのつなぎ方、部品に渡す間隔や時間の値（認証情報の発行し直し、
  片付け、Slack のトークンの確かめの間隔など。値の意味はそれを使う部品の文書にある。部品の中で閉じる値は部品が
  持つ）、複数のアダプターを一つのポートにまとめる小さな型（`credentialIssuers`、`sessionObserver`）。`Dispatcher` と Bot が
  互いを要る循環は、`sessionObserver` に Bot を後から渡すことでここだけで解く。判定は置かない。
- **関係**：すべてのパッケージに依存してよい。起動は、自分のバイナリを `tools.dir` に置き（失敗したら起動しない。
  [作業領域を読むコンテナ](containers.md#workspace-reader "在りか")）、`Dispatcher` を作り、Bot を作って `sessionObserver` に渡し、
  `Dispatcher.Recover` で前の実行が残したコンテナを消してから、HTTP サーバー、Bot、片付け、Slack のトークンの
  確かめ（要れば更新）を動かす。止めるときは、Bot が依頼を受けるのをやめ、`Dispatcher` を閉じて動いていた処理を中断し
  （[停止と再起動の知らせ](requests.md#shutdown "在りか")）、HTTP サーバーを 10 秒の期限で止め、片付けと Bot の
  書き込みが終わるのを待つ。
- **当てはめた原則・パターン**：[層に分けた構成](../../../platform/plugins/principles/docs/reference/patterns.md#layered-architecture)：
  部品を組み立てる場所で、ここだけが全部を知る。ユースケースはアダプターを知らない（層は[コードの構成](../codebase/layers.md "在りか")）。

### `cmd/acp`（`main.go`、`run.go`、`workspace_state.go`、`cleanup_check.go`、`slackauth.go`）

- **責務**：サブコマンドの振り分けと、HTTP API のクライアントとしての `acp run` と `acp stop`。`acp workspace-state` は、
  作業ディレクトリの作業領域の状態を JSON で出す（[作業領域の状態](cleanup.md#state "在りか")。control-plane が
  作業領域を読むコンテナの中で動かす）。`acp cleanup-check`（`cleanup_check.go`）は、エージェントのコンテナの中で、
  そのセッションが片付けで残される理由を出す（usecase の [CheckCleanup](cleanup.md#check-cleanup "在りか")）。`acp slack-auth`（`slackauth.go`）は、
  Slack のトークンを取り直す（[認証情報](credentials.md#slack-auth "在りか")）。
- **関係**：`run` と `stop` は `httpapi.Client` を使う。使い方は[依頼の仕方](../../reference/usage.md "在りか")にある。

### `internal/usecase/ports.go`

- **責務**：ユースケースが外部に求めるインターフェースと、それに渡す値の型。
- **置くもの / 置かないもの**：チャットの入口に関わるポート（`AccessPolicy`、`ChatResponder`、`TurnExecutor`、`SessionCleanup`）だけは
  ここに置かず、それを使う `handle_message.go` に置く（上の表）。
- **当てはめた原則・パターン**：[層に分けた構成](../../../platform/plugins/principles/docs/reference/patterns.md#layered-architecture)：
  中心が外側に要るものをポートとして決め、アダプターはここに合わせて作る。

### `internal/adapter/config`

- **責務**：設定ファイル（`acp.yaml`）を読み、検証して、エージェントの一覧、依頼者の許可、依頼者とコミットの
  作者の対応を返す。
- **置くもの / 置かないもの**：ファイルの形、既定値、読むときの検証。設定の項目の説明は
  [acp.example.yaml](../../../acp.example.yaml "在りか") に置く。
- **関係**：`AgentRegistry`、`AccessPolicy`、`People` を満たす。
- **当てはめた原則・パターン**：[仕組みで守る](../../../platform/plugins/principles/docs/reference/principles.md#mechanisms)：知らない項目や
  相対パスは、起動の前に読むところで弾く。
