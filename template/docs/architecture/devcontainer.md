# devcontainer

Anthropic の [Claude Code devcontainer](https://github.com/anthropics/claude-code/tree/main/.devcontainer) を元にした、
人の開発環境とエージェントの実行環境を兼ねるコンテナ（`.devcontainer/`）。

使い方は [guides/devcontainer.md](../guides/devcontainer.md) にある。

## 方針

- コンテナの振る舞いは Dockerfile、compose.yaml、entrypoint で決める。devcontainer.json は VS Code 側の
  配線だけを持つ（[ADR 0001](../adr/0001-devcontainer-independent-of-vscode.md)）。
- `base` ステージには、リポジトリを問わずエージェントが使うもの（Node、Python と uv、シェルツール、Claude Code など）
  だけを入れ、リポジトリやホストに依存する値（リポジトリ名、タイムゾーン、UID）を含めない。層は変わりにくいものから
  順に並べる。リポジトリ固有のツールチェーンとホストに依存する値は後段のステージに置く。
- RUN は `pipefail` 付きの bash で動かし、パイプの途中の失敗（ダウンロードの失敗など）でビルドを止める。
- 外から取得するもの（ベースイメージ、Claude Code、ツール）の版は Dockerfile で固定する。上げるときは PR で変え、
  CI でイメージのビルドと検査を確かめる。Claude Code はネイティブのインストーラーで入れ、自分で更新しないよう
  `DISABLE_UPDATES=1` にする。
- `node` には sudo を与えない。apt のパッケージ一覧はイメージに残し、人が `docker exec -u root` で入って一時的に
  インストールできるようにする（[ADR 0002](../adr/0002-no-sudo-for-agent-user.md)）。コンテナの中にファイアウォールは
  置かない（[ADR 0008](../adr/0008-no-in-container-firewall.md)）。

## 構成

- devcontainer のイメージは `base` → `go`（このリポジトリのツールチェーンとホストに依存する値）で、
  `<リポジトリ名>-devcontainer` という名前でビルドされる。control-plane はエージェントも同じイメージで動かす
  （`acp.yaml` の `target.image`）。Go のツールは base と切り離した `go-tools` ステージでビルドし、バイナリだけを
  `go` にコピーする。base を変えてもツールを作り直さない。CI 用の `check-*` ステージは [ci.md](ci.md) にある。
- ビルドの文脈は `.devcontainer/Dockerfile.dockerignore` で決める（ルートの `.dockerignore` は control-plane の
  イメージ用）。
- `..` を `/workspace/<リポジトリ名>` に読み書き可、`../..` を `/workspace` に**読み取り専用**で
  マウントする。隣に置いたリポジトリは参照できるが書き込めない。
- Claude Code の設定（ログイン、履歴、メモリ）、`gh` の設定、シェルの履歴は、ホストの `DEV_STATE_DIR` の下に置いて
  マウントする。Docker の外にあるので、`docker compose down -v` や prune でも消えない
  （[ADR 0010](../adr/0010-devcontainer-state-on-the-host.md)）。VS Code のサーバー（拡張機能とチャットの履歴を含む）は、コンテナを作り直しても残るよう名前付きボリュームに置く。
  Go のキャッシュは消えても困らないので、名前付きボリュームに置く。
- コンテナは root を使わず、数値の UID と GID の `node` で動く（[ADR 0009](../adr/0009-containers-run-as-non-root.md)）。
  UID と GID は、ビルドのときにリポジトリの所有者の値（`.devcontainer/.env`）に合わせる。control-plane の
  `.env` の `ACP_UID` / `ACP_GID` も同じ値にする。ボリュームのマウント先はイメージの中に `node` の持ち物として
  作ってあり、新しい名前付きボリュームはその所有者を引き継ぐ。root での作業の仕方は[使い方](../guides/devcontainer.md#root)にある。

## Claude Code の設定

`.claude/settings.json` は人が対話で使うときの設定。エージェントは `--dangerously-skip-permissions` で動くので、
`permissions` は効かない。コンテナの隔離で守る方針なので、対話でも権限の確認を省いて（bypass）使ってよい。
`permissions.allow` の一覧は、組織の制限などで bypass が使えない環境で確認を減らすためにある。

## ターミナル

- VS Code の既定のターミナルは tmux。VS Code やリモートの接続が切れても、中で動いている Claude Code は止まらない。
- tmux のコピーモードは普段の端末の選択に近づけている。マウスでの選択（ドラッグ、ダブルクリック、トリプルクリック）は
  選択するだけで自動ではコピーせず、`y`、`Enter`、`Ctrl+C` でコピーする。`Esc` かクリックで抜ける。
- Claude Code のフルスクリーン表示では、マウスでの選択を Claude Code が扱い、離したときに自動でコピーする。
  止め方は[使い方](../guides/devcontainer.md#copy)にある。
- Claude Code が対話で止まったときは、platform の bell プラグインがベルを鳴らす（[bell](../../../platform/docs/architecture/plugins.md#bell)）。

## VS Code の設定

エディタ設定は `devcontainer.json` の `customizations.vscode.settings`（コンテナに attach したウィンドウ）と
`.vscode/settings.json`（attach していないウィンドウ）に同じ内容で置く。ベル設定のように、片方だけだと
効いたり効かなかったりするものがあるため。同じであることは `make check-target` で確かめる。
