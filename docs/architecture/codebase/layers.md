# コードの構成

control-plane の Go のコードは、[層に分けた構成](../../../platform/plugins/principles/docs/reference/patterns.md#layered-architecture)をとり、依存の向きを内側へそろえた層に分ける（[ADR 0001](../../adr/codebase/0001-layered-architecture.md)）。
書き方の決まりは [reference/coding.md](../../reference/coding.md "在りか") にある。

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
- **当てはめた原則・パターン**：[層に分けた構成](../../../platform/plugins/principles/docs/reference/patterns.md#layered-architecture)：
  中心なので外側を知らない。

### `internal/usecase`

- **責務**：ユースケースと、それが外部に求めるインターフェース（ポート）。
- **置くもの / 置かないもの**：判定と手順。ユースケースごとに 1 ファイルで、一つの型がそのユースケースの操作を
  まとめて持つ。ポートは `ports.go` か、それを使うユースケースのファイルに置く
  （どのポートをどちらに置くかは[control-plane の構成](../app/components.md "在りか")にある）。外部の技術の型は置かない。
- **関係**：標準ライブラリと domain に依存する。
- **当てはめた原則・パターン**：[層に分けた構成](../../../platform/plugins/principles/docs/reference/patterns.md#layered-architecture)：
  外側はここが決めたポートを実装する。
  [単一責任](../../../platform/plugins/principles/docs/reference/principles.md#srp)：ファイルをユースケースごとに分ける。

### `internal/adapter/<技術>`

- **責務**：usecase のポートを、一つの技術で実装する。
- **置くもの / 置かないもの**：外部（ファイル、git、Docker、GitHub、Slack など）を読み書きし、事実を返すこと。判定は置かない。
  一つのアダプターは一つのパッケージにし、下にパッケージを作らない。
- **関係**：上記に加えて外部ライブラリに依存する。ほかのアダプターには依存せず、アダプターどうしをつなぐのは
  `cmd/acp`。depguard は、試験のファイルを除いて、`internal/adapter` の下のほかのパッケージ（自分の下に
  作ったものも）への依存を断る。
- **当てはめた原則・パターン**：[層に分けた構成](../../../platform/plugins/principles/docs/reference/patterns.md#layered-architecture)：
  外側の層。[関心の分離](../../../platform/plugins/principles/docs/reference/principles.md#soc)：技術ごとにパッケージを分ける。
  [仕組みで守る](../../../platform/plugins/principles/docs/reference/principles.md#mechanisms)：アダプターとパッケージの境界をそろえ、
  アダプターどうしの依存を、パッケージの名前の接頭辞だけで見分けて一つの規則で断れるようにする。

### `cmd/acp`

- **責務**：部品を組み立てる場所。ファイルごとの中身は[control-plane の構成](../app/components.md "在りか")にある。
- **関係**：すべてに依存してよい。
- **当てはめた原則・パターン**：[層に分けた構成](../../../platform/plugins/principles/docs/reference/patterns.md#layered-architecture)：
  組み立てる場所だけが全部を知る。
