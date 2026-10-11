# app

control-plane というアプリケーションの今の姿。依頼の仕方は [usage.md](usage.md)、運用の手順は [operations.md](operations.md)、
判断の理由は [adr/](adr/) にある。

## セッションのライフサイクル

セッションと作業領域の意味は[用語集](../../platform/docs/glossary.md)のとおり。エージェントから見たセッション（渡されるもの、コンテナの寿命）は[ハーネスの契約](../../platform/docs/harness.md)にある。

| 段階 | 起きること |
| --- | --- |
| ID が決まる | Slack では、エージェントとスレッド（ワークスペース、チャンネル、スレッドの ts）から UUIDv5 で決まる。同じスレッドなら何度でも同じ ID になる（[ADR 0016](adr/0016-chat-entry-point.md)）。CLI では新しいセッションごとにランダム |
| 作る | 記録（`<sessions.root>/<ID>/session.json`）がなければ、ロックを取って作る。ターゲットを `<sessions.root>/<ID>/workspace/<リポジトリ名>` に clone し、ブランチ `acp/<ID の先頭8文字>` をリモートのデフォルトブランチ（GitHub App がなければ元のリポジトリの HEAD）から切る（[ADR 0010](adr/0010-per-session-cloned-workspace.md)、[0013](adr/0013-session-branch-from-remote-default.md)） |
| 動かす | 依頼が来るとセッションのワーカーができ、ロック（`<ID>/.lock` の flock）を取る。コンテナがなければ起動し、Claude Code を、終えたターンが 0 なら `--session-id <ID>`、それ以外は `--resume <ID>` で動かし続ける（[ADR 0017](adr/0017-long-lived-session-process.md)）。ターンを終えるたびに記録の `Turns` を増やして保存する |
| 休む | ターンも待っている依頼もバックグラウンドの処理もない状態が `sessions.idle_timeout` 続くと、コンテナを片付けてロックを放す。記録、作業領域、会話の履歴は残り、次の依頼で `--resume` する |
| 中断する | control-plane の再起動で、動いていた処理は中断され、コンテナは片付けられ、スレッドにそう知らせる（[ADR 0024](adr/0024-restart-abandons-running-work.md)）。次の依頼で `--resume` する |
| 片付ける | 使われないまま `sessions.retention`（既定 30 日）を過ぎたセッションを、control-plane が起動時と 1 時間ごとに消す。最後に使った時刻は記録の `last_used_at`。続きがありうる（開いている PR がある）か、消すと作業が失われる（作業ツリーの変更、GitHub のどこにもないコミットなど）なら残し、保持期間ごとに 1 回スレッドで片付けを促す。消すときはスレッドに何を消したかを知らせる（[ADR 0026](adr/0026-clean-up-unused-sessions.md)） |

セッションに属するものの置き場所：

| もの | 場所 |
| --- | --- |
| 記録とロック | `<sessions.root>/<ID>/` |
| 作業領域 | `<sessions.root>/<ID>/workspace/<リポジトリ名>`（コンテナでは `/workspace/<リポジトリ名>`） |
| 会話の履歴 | エージェントの Claude Code の設定ディレクトリ（`acp-claude` ボリュームの `<エージェント名>/`）の `projects/` の下。作業ディレクトリのパスと ID で決まる |
| 認証情報 | `acp-credentials` ボリュームの `<ID>/` |
| ブランチと PR | セッションが作るのはリモートの `acp/<ID の先頭8文字>` と、それに `-<内容>` を付けたブランチ。ほかのブランチにも push することがあり、どれに push したかは作業領域の git の記録から分かる（[ADR 0026](adr/0026-clean-up-unused-sessions.md)） |

### 壊してはいけない前提

セッションに関わる変更では、次の前提が崩れないことを確かめる。

- **記録が消えても ID は変わらない。** スレッドから ID が決まるので、記録だけを消すと、次の依頼は同じ ID で新しいセッションを作る。そのとき、同じ ID の会話の履歴が残っていると `--session-id` が使えず、同じ名前のブランチがリモートに残っていると push がぶつかる。記録、会話の履歴、ブランチは一緒に扱う。
- **作業ディレクトリのパスはセッションの間変わらない。** 会話の履歴は作業ディレクトリのパスで引かれるので、変わると `--resume` できない。
- **`Turns` は `--session-id` と `--resume` の切り替えに使う。** ターンを終えたのに保存されないと、次の起動で同じ ID の会話を新しく始めようとする。
- **セッションのディレクトリを触るときはロックを取る。** ロックはセッションを作る間と、ワーカーが動いている間（休むまで）持たれ、別の control-plane や片付けの処理と同時に触らないためにある。
