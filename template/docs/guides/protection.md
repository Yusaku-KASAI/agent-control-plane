# 既定のブランチの守りの用意

ターゲットのリポジトリで、既定のブランチへの変更を PR 経由に限り、マージに数える承認を人に限る手順。何を守るか
は[ターゲットの契約](../../../platform/docs/reference/target.md#protection "在りか")に、理由は [app ADR 0027](../../../docs/adr/app/0027-require-human-approval-in-ruleset.md)
と [ADR 0015](../adr/0015-codeowners-by-default.md) にある。

<a id="approvers"></a>

## 承認する人を決める

リポジトリの持ち主で分かれる。どちらでも、エージェントの GitHub App は持ち主にも team にも入れない。

- **個人のリポジトリ**：`.github/CODEOWNERS` の `* @<ユーザー名>` に、持ち主（人）の GitHub のユーザー名を書く。
  その人はリポジトリに書き込める権限を持つ。
- **organization のリポジトリ**：`.github/CODEOWNERS` を消す。人だけの team を作り、リポジトリへの書き込みの権限を
  与える。

<a id="ruleset"></a>

## ルールセットを作る

リポジトリの Settings → Rules → Rulesets で、ブランチのルールセットを次のとおりに作る。

| 項目 | 値 |
| --- | --- |
| Enforcement status | Active |
| Target branches | Include default branch |
| Bypass list | Repository admin だけを、For pull requests only で足す |
| Restrict deletions、Block force pushes | オン |
| Require a pull request before merging | オン。Required approvals は 1、Dismiss stale pull request approvals when new commits are pushed をオン |
| 承認する人（個人のリポジトリ） | Require review from Code Owners をオン |
| 承認する人（organization のリポジトリ） | Required reviewers に上の team を足し、ファイルのパターンは `*`、必須の承認数は 1 にする。承認数が 0 だと team は通知されるだけで、承認は求められない |
| Require status checks to pass | オン。必須のチェックは `check` だけで、報告する元は GitHub Actions にする（[CI](../architecture/ci.md "在りか")）。元を固定しないと、コミットの状態を書ける誰でも `check` の成功を付けられる |

ルールセットの `require_extra_approval_for_unattributed_changes` は、オンでもオフでもよい。この守りには数えない。

<a id="verify"></a>

## 確かめる

github プラグインの `check-protection` が、契約の守り（上の設定はそれを満たす）と CODEOWNERS を確かめる。ターゲットの CI の `check-protection`
ジョブが毎回回すので（[CI](../architecture/ci.md "在りか")）、守りを用意するまでは、そのジョブと、それを待つ必須のチェックの
`check` が落ちる。設定したあとは CI を回し直して、すべてのジョブが通ることを見る。手元では
リポジトリの中で回す（エージェントは `check-protection`、人は platform の `plugins/github/bin/check-protection`）。

```bash
check-protection [<owner>/<repo> [<ref>]]   # NG の行が守りの抜け。終了コードは 0 が通過、1 が抜け、2 が読めない
```

バイパスの設定（`bypass_actors`）は、ルールセットを書ける認証情報でしか読めず、CI では確かめない。管理者の認証情報で
回すか、設定の画面で確かめる。
