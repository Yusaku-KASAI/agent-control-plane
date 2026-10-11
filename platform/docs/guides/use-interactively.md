# 対話で使う準備

devcontainer の Claude Code で、ターゲットが有効にした platform のプラグインを使えるようにする。最初に一度
マーケットプレイスを登録し、ターゲットの `.claude/settings.json` の `enabledPlugins` にあるものを入れる。

```bash
claude plugin marketplace add /workspace/agent-platform
claude plugin install roles@acp-platform   # enabledPlugins のそれぞれ
```

`/workspace/agent-platform` には、ターゲットのホストの準備で platform が置かれている。

プラグインを変えたら、`/reload-plugins` で読み込み直す。hook の設定は起動時に読まれ、hook が呼ぶスクリプトは呼ばれる
たびに読まれる。
