# Claude Code を対話で使う

devcontainer の中で、人が Claude Code を対話で使うときの手順。設定の構成は
[architecture/claude-code.md](../architecture/claude-code.md "在りか") にある。

<a id="permissions"></a>

## 権限の確認

対話の既定のモードと、確認せずに使えるツールの一覧は `.claude/settings.json` の `permissions` にある。コンテナで
隔離しているので、確認を省いて使ってもよい。

```bash
claude --dangerously-skip-permissions
```
