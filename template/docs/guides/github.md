# 既定のブランチの守りの用意

ターゲットのリポジトリで、既定のブランチへの変更を PR 経由に限り、マージに数える承認を人に限る手順。何を守るか
は[ターゲットの契約](../reference/contract.md#protection "在りか")に、理由は [app ADR 0027](../../../docs/adr/app/0027-require-human-approval-in-ruleset.md)
と [ADR 0014](../adr/0014-codeowners-by-default.md) にある。

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
| 承認する人（organization のリポジトリ） | Required reviewers に、上の team を指定する |
| Require status checks to pass | オン。必須のチェックは `check` だけ（[CI](../architecture/ci.md "在りか")） |

ルールセットの `require_extra_approval_for_unattributed_changes` は、オンでもオフでもよい。この守りには数えない。

<a id="verify"></a>

## 確かめる

CODEOWNERS を置いたときと変えたあとは、誤りがないことを確かめる。誤った行は黙って無視され、コードオーナーの承認が
求められなくなる。

```bash
gh api repos/<owner>/<repo>/codeowners/errors   # {"errors":[]} なら誤りはない
gh api repos/<owner>/<repo>/rulesets            # ルールセットの id
gh api repos/<owner>/<repo>/rulesets/<id>       # rules の pull_request と required_status_checks を見る
```

ルールセットの `bypass_actors` は、読む認証情報の権限によっては空（`null`）で返る。バイパスの設定は画面で確かめる。
