# コードの構成

control-plane の Go のコードは、[層に分けた構成](../../../platform/docs/reference/patterns.md#layered-architecture)をとり、依存の向きを内側へそろえた層に分ける（[ADR 0001](../../adr/codebase/0001-layered-architecture.md)）。
書き方の決まりは [reference/coding.md](../../reference/coding.md) にある。

## 構成

```
cmd/acp ──▶ internal/adapter/<技術> ──▶ internal/usecase ──▶ internal/domain
（組み立て）  （技術ごとの実装）          （ユースケースとポート）  （エンティティ）
```

依存の向きは golangci-lint の depguard で検査する（`.golangci.yml`）。

## 構成要素

### `internal/domain`

- **責務**：エンティティと値の検証。
- **関係**：標準ライブラリだけに依存する。
- **当てはめた原則・パターン**：[依存の向き](../../../platform/docs/reference/principles.md#dependency-rule)：中心なので外側を知らない。

### `internal/usecase`

- **責務**：1 操作 1 ファイルのユースケースと、それが使うインターフェース（`ports.go`）。
- **置くもの / 置かないもの**：判定と手順。外部の技術の型は置かない。
- **関係**：標準ライブラリと domain に依存する。
- **当てはめた原則・パターン**：[依存の向き](../../../platform/docs/reference/principles.md#dependency-rule)：外側はここが決めたインターフェースを実装する。
  [単一責任](../../../platform/docs/reference/principles.md#srp)：1 ファイル 1 操作。

### `internal/adapter/<技術>`

- **責務**：usecase のインターフェースを、一つの技術で実装する。
- **置くもの / 置かないもの**：外部（ファイル、git、Docker、GitHub、Slack など）を読み書きし、事実を返すこと。判定は置かない。
- **関係**：上記に加えて外部ライブラリに依存する。
- **当てはめた原則・パターン**：[依存の向き](../../../platform/docs/reference/principles.md#dependency-rule)、[関心の分離](../../../platform/docs/reference/principles.md#soc)：技術ごとにパッケージを分ける。

### `cmd/acp`

- **責務**：CLI と、各部品の組み立て。
- **関係**：すべてに依存してよい。
