# コードベース

## 層と依存の向き

| パッケージ | 役割 | 依存してよいもの |
| --- | --- | --- |
| `internal/domain` | エンティティと値の検証 | 標準ライブラリ |
| `internal/usecase` | 1 操作 1 ファイルのユースケースと、それが使うインターフェース（`ports.go`） | 標準ライブラリ、domain |
| `internal/adapter/<技術>` | usecase のインターフェースの実装 | 上記に加えて外部ライブラリ |
| `cmd/acp` | CLI と、各部品の組み立て | すべて |

依存の向きは golangci-lint の depguard で検査する（`.golangci.yml`）。

## 書き方

- エラーは `error` を返し、`fmt.Errorf("...: %w", err)` で文脈を足す。呼び出し側が判別するエラーは
  `var ErrXxx = errors.New(...)` として定義し、`errors.Is` で判定する。
- テストは標準の `testing` でテーブル駆動にする。インターフェースの偽物はテストファイルに手書きする。

## コマンド

```bash
make check-go   # golangci-lint、go mod tidy の差分、govulncheck、go test -race
make fmt        # 整形
```
